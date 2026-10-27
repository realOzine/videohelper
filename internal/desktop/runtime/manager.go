package runtime

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/mlclient"
)

type Progress struct {
	Stage    string   `json:"stage"`
	Message  string   `json:"message"`
	Fraction *float64 `json:"fraction,omitempty"`
}

type State struct {
	Stage    string   `json:"stage"`
	Message  string   `json:"message"`
	Fraction *float64 `json:"fraction,omitempty"`
	Device   string   `json:"device,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Error    string   `json:"error,omitempty"`
	Ready    bool     `json:"ready"`
}

type Services struct {
	Endpoints                                           []string
	VisualSpace, AudioSpace, Transcriber, Token, Device string
}

type Manager struct {
	dir                                             string
	assets                                          fs.FS
	op                                              sync.Mutex
	mu                                              sync.Mutex
	state                                           State
	tools                                           Tools
	uv, root, python, visualProfile, visualManifest string
	services                                        Services
	processes                                       []*ownedProcess
	active                                          map[*ownedProcess]struct{}
	ensureDone                                      chan struct{}
	cancel                                          context.CancelFunc
	closed                                          bool
}

func New(dataDir string, assets fs.FS) *Manager {
	if absolute, err := filepath.Abs(dataDir); err == nil {
		dataDir = absolute
	}
	return &Manager{dir: dataDir, assets: assets, active: make(map[*ownedProcess]struct{}), state: State{Stage: "pending", Message: "正在准备本地运行时"}}
}

func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.state
	if s.Fraction != nil {
		f := *s.Fraction
		s.Fraction = &f
	}
	return s
}
func (m *Manager) Tools() Tools { m.mu.Lock(); defer m.mu.Unlock(); return m.tools }
func (m *Manager) update(stage, message string, notify func(Progress)) {
	m.mu.Lock()
	m.state.Stage = stage
	m.state.Message = message
	m.state.Error = ""
	m.state.Fraction = nil
	m.mu.Unlock()
	if notify != nil {
		notify(Progress{Stage: stage, Message: message})
	}
}
func (m *Manager) failed(err error) error {
	m.mu.Lock()
	m.state.Stage = "failed"
	m.state.Error = err.Error()
	m.state.Ready = false
	m.mu.Unlock()
	return err
}

func (m *Manager) Ensure(ctx context.Context, notify func(Progress)) (err error) {
	m.op.Lock()
	defer m.op.Unlock()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return errors.New("runtime is closed")
	}
	ctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	done := make(chan struct{})
	m.ensureDone = done
	m.mu.Unlock()
	defer close(done)
	defer cancel()
	defer func() {
		if err != nil {
			m.failed(err)
		}
	}()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	platform := runtime.GOOS + "/" + runtime.GOARCH
	components, err := componentsFor(platform)
	if err != nil {
		return err
	}
	if platform == "darwin/amd64" {
		return errors.New("此 ML profile 尚不支持 Intel Mac：当前固定 Torch 未提供 Intel macOS wheel；请使用 Apple Silicon 或 Linux/Windows x64")
	}
	if err = os.MkdirAll(filepath.Join(m.dir, "runtime"), 0700); err != nil {
		return err
	}
	unlock, err := lockRuntime(ctx, filepath.Join(m.dir, "runtime", "install.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	m.update("extracting", "正在释放内置模型服务", notify)
	m.root, err = extractAssets(m.assets, filepath.Join(m.dir, "runtime"))
	if err != nil {
		return err
	}
	paths := make(map[string]string)
	for _, component := range components {
		m.update("tools", "正在准备 "+component.Name, notify)
		paths[component.Name], err = installComponent(ctx, downloadClient(), filepath.Join(m.dir, "runtime", "tools"), component)
		if err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.tools = Tools{FFmpeg: paths["ffmpeg"], FFprobe: paths["ffprobe"], YTDLP: paths["yt-dlp"], Deno: paths["deno"]}
	m.mu.Unlock()
	m.uv = paths["uv"]
	metadata := filepath.Join(m.root, "python-downloads.json")
	if err = os.WriteFile(metadata, pythonDownloads, 0600); err != nil {
		return err
	}
	m.update("python", "正在准备私有 Python 3.12.15", notify)
	if _, err = m.run(ctx, m.uv, []string{"python", "install", "cpython-3.12.15", "--no-bin", "--no-registry", "--python-downloads-json-url", metadata}, nil); err != nil {
		return err
	}
	output, err := m.run(ctx, m.uv, []string{"python", "find", "cpython-3.12.15", "--managed-python", "--no-python-downloads"}, nil)
	if err != nil {
		return err
	}
	m.python = strings.TrimSpace(output)
	if !filepath.IsAbs(m.python) {
		return errors.New("uv returned a non-absolute Python path")
	}
	m.visualProfile = "wemm-cpu"
	if platform == "darwin/arm64" {
		m.visualProfile = "wemm-mps"
	} else {
		// Driver presence only chooses which wheels to try; Torch must still prove the device works.
		output, hintErr := m.run(ctx, m.python, []string{filepath.Join(m.root, "ml", "app", "hardware.py"), "--cuda-candidate"}, nil)
		if hintErr == nil && strings.TrimSpace(output) == "cuda" {
			m.visualProfile = "wemm-cuda"
		}
	}
	var fallbackReason string
	for _, profile := range []string{m.visualProfile, "speech-audio-cpu"} {
		m.update("packages", "正在安装固定依赖："+profile, notify)
		if err = m.installProfile(ctx, profile); err != nil {
			if profile != "wemm-cuda" || ctx.Err() != nil {
				return err
			}
			m.update("packages", "CUDA 运行时安装失败，正在准备 CPU 运行时", notify)
			fallbackReason = "CUDA 运行时安装失败，使用 CPU"
			m.visualProfile = "wemm-cpu"
			if err = m.installProfile(ctx, m.visualProfile); err != nil {
				return err
			}
		}
	}
	m.update("detecting", "正在用 PyTorch 实际运算检测 MPS / CUDA / CPU", notify)
	output, err = m.run(ctx, m.profilePython(m.visualProfile), []string{filepath.Join(m.root, "ml", "app", "hardware.py")}, nil)
	if err != nil && m.visualProfile == "wemm-cuda" && ctx.Err() == nil {
		fallbackReason = "CUDA Torch 运行失败，使用 CPU"
		m.visualProfile = "wemm-cpu"
		m.update("packages", fallbackReason, notify)
		if err = m.installProfile(ctx, m.visualProfile); err != nil {
			return err
		}
		output, err = m.run(ctx, m.profilePython(m.visualProfile), []string{filepath.Join(m.root, "ml", "app", "hardware.py")}, nil)
	}
	if err != nil {
		return err
	}
	var probe struct {
		Backend string `json:"backend"`
		Reason  string `json:"reason"`
	}
	if err = json.Unmarshal([]byte(strings.TrimSpace(output)), &probe); err != nil {
		return fmt.Errorf("decode Torch probe: %w", err)
	}
	if fallbackReason != "" {
		probe.Reason = fallbackReason + "; " + probe.Reason
	}
	var space string
	switch probe.Backend {
	case "mps":
		m.visualManifest = "wemm-mps"
		space = "wemm-2b@r1"
	case "cuda":
		m.visualManifest = "wemm-cuda"
		space = "wemm-2b-cuda-experimental@r1"
	case "cpu":
		m.visualManifest = "wemm-cpu"
		space = "wemm-2b-cpu@r1"
	default:
		return errors.New("Torch probe returned an unsupported backend")
	}
	m.mu.Lock()
	m.state.Device = probe.Backend
	m.state.Reason = probe.Reason
	m.services = Services{VisualSpace: space, AudioSpace: "clap-htsat-unfused@r1", Transcriber: "paraformer-zh@r1", Device: probe.Backend}
	m.mu.Unlock()
	for _, pair := range [][2]string{{m.visualProfile, m.visualManifest}, {"speech-audio-cpu", "speech-audio-cpu"}} {
		m.update("models", "正在准备固定模型权重："+pair[1], notify)
		args := []string{"-m", "app.cli", "prepare", "--models", m.manifest(pair[1])}
		if _, err = m.run(ctx, m.profilePython(pair[0]), args, []string{"HF_HUB_OFFLINE=1"}); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if _, err = m.run(ctx, m.profilePython(pair[0]), args, []string{"HF_HUB_OFFLINE=0"}); err != nil {
				return err
			}
		}
	}
	m.update("installed", "本地运行时已准备完成，设备："+probe.Backend+" "+probe.Reason, notify)
	return nil
}

func (m *Manager) profile(name string) string { return filepath.Join(m.root, "ml", "profiles", name) }
func (m *Manager) installProfile(ctx context.Context, name string) error {
	_, err := m.run(ctx, m.uv, []string{"sync", "--project", m.profile(name), "--frozen", "--no-dev", "--no-default-groups", "--no-editable", "--python", m.python}, nil)
	return err
}
func (m *Manager) manifest(name string) string { return filepath.Join(m.profile(name), "models.yaml") }
func (m *Manager) profilePython(profile string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(m.profile(profile), ".venv", "Scripts", "python.exe")
	}
	return filepath.Join(m.profile(profile), ".venv", "bin", "python")
}

func (m *Manager) environment(extra []string) []string {
	// Environment selection must not inherit a caller's fake models, Python path or active venv.
	keys := map[string]bool{"PYTHONPATH": true, "PYTHONHOME": true, "VIRTUAL_ENV": true, "CONDA_PREFIX": true}
	env := make([]string, 0)
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if !keys[key] && !strings.HasPrefix(key, "ML_") && !strings.HasPrefix(key, "UV_") && !strings.HasPrefix(key, "HF_") {
			env = append(env, item)
		}
	}
	env = append(env, "UV_PYTHON_INSTALL_DIR="+filepath.Join(m.dir, "runtime", "python"), "UV_CACHE_DIR="+filepath.Join(m.dir, "runtime", "uv-cache"), "UV_MANAGED_PYTHON=1", "UV_NO_ENV_FILE=1", "PYTHONNOUSERSITE=1", "PYTHONUNBUFFERED=1", "HF_HOME="+filepath.Join(m.dir, "models"), "ML_FAKE=0", "ML_LOCAL_MEDIA_ROOT="+filepath.Join(m.dir, "media"))
	env = append(env, extra...)
	return env
}

func (m *Manager) run(ctx context.Context, path string, args, extra []string) (string, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = m.environment(extra)
	cmd.Dir = m.root
	var output, stderr tailBuffer
	cmd.Stdout = &output
	cmd.Stderr = &stderr
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return "", errors.New("runtime is closed")
	}
	p, err := startOwned(ctx, cmd)
	if err == nil {
		m.active[p] = struct{}{}
	}
	m.mu.Unlock()
	if err != nil {
		return "", err
	}
	<-p.done
	m.mu.Lock()
	delete(m.active, p)
	m.mu.Unlock()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if p.err != nil {
		return "", fmt.Errorf("%s failed: %w: %s", filepath.Base(path), p.err, stderr.String())
	}
	return output.String(), nil
}

func extractAssets(assets fs.FS, base string) (string, error) {
	if assets == nil {
		return "", errors.New("runtime assets are missing; build with make desktop-assets")
	}
	h := sha256.New()
	files := make(map[string][]byte)
	err := fs.WalkDir(assets, "ml", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() || !fs.ValidPath(path) {
			return fmt.Errorf("invalid runtime asset %s", path)
		}
		data, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		files[path] = data
		fmt.Fprintf(h, "%s\x00%d\x00", path, len(data))
		h.Write(data)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("read packaged ML assets: %w", err)
	}
	if len(files) == 0 {
		return "", errors.New("packaged ML assets are empty")
	}
	root := filepath.Join(base, "ml-"+hex.EncodeToString(h.Sum(nil))[:20])
	if _, err := os.Stat(filepath.Join(root, ".assets-ready")); err == nil {
		return root, nil
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	for path, data := range files {
		dst := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
			return "", err
		}
		if err := os.WriteFile(dst, data, 0600); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".assets-ready"), []byte("1"), 0600); err != nil {
		return "", err
	}
	return root, nil
}

func (m *Manager) Start(ctx context.Context) (services Services, err error) {
	m.op.Lock()
	defer m.op.Unlock()
	defer func() {
		if err != nil {
			m.failed(err)
			stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			m.Close(stopCtx)
		}
	}()
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return Services{}, errors.New("runtime is closed")
	}
	if len(m.processes) > 0 {
		if !m.state.Ready {
			m.mu.Unlock()
			return Services{}, errors.New("ML service is no longer ready")
		}
		services = m.services
		m.mu.Unlock()
		return services, nil
	}
	if m.visualManifest == "" {
		m.mu.Unlock()
		return Services{}, errors.New("runtime Ensure must complete before Start")
	}
	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.services.Token = rand.Text()
	m.mu.Unlock()
	m.update("loading", "正在加载本地模型", nil)
	if err = os.MkdirAll(filepath.Join(m.dir, "media"), 0700); err != nil {
		return Services{}, err
	}
	var endpoints []string
	for _, pair := range [][2]string{{m.visualProfile, m.visualManifest}, {"speech-audio-cpu", "speech-audio-cpu"}} {
		listener, listenErr := net.Listen("tcp4", "127.0.0.1:0")
		if listenErr != nil {
			return Services{}, listenErr
		}
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		endpoint := fmt.Sprintf("http://127.0.0.1:%d", port)
		cmd := exec.CommandContext(runCtx, m.profilePython(pair[0]), "-m", "app.cli", "serve", "--models", m.manifest(pair[1]), "--host", "127.0.0.1", "--port", fmt.Sprint(port))
		cmd.Dir = m.root
		cmd.Env = m.environment([]string{"ML_INTERNAL_TOKEN=" + m.services.Token, "ML_CACHE_DIR=" + filepath.Join(m.dir, "runtime", "media-cache", pair[0]), "HF_HUB_OFFLINE=1"})
		var logs tailBuffer
		cmd.Stdout = &logs
		cmd.Stderr = &logs
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return Services{}, errors.New("runtime is closed")
		}
		process, startErr := startOwned(runCtx, cmd)
		if startErr != nil {
			m.mu.Unlock()
			return Services{}, startErr
		}
		m.processes = append(m.processes, process)
		m.mu.Unlock()
		if err = waitReady(runCtx, endpoint, process, &logs); err != nil {
			return Services{}, err
		}
		endpoints = append(endpoints, endpoint)
	}
	client := mlclient.New(config.MLConfig{Endpoints: endpoints, Token: m.services.Token}, nil)
	for i, endpoint := range endpoints {
		metadata, discoverErr := client.Spaces(runCtx, endpoint)
		if discoverErr != nil {
			return Services{}, discoverErr
		}
		found := false
		for _, space := range metadata.Spaces {
			if (i == 0 && space.ID == m.services.VisualSpace) || (i == 1 && space.ID == m.services.AudioSpace) {
				found = true
			}
		}
		if !found {
			return Services{}, fmt.Errorf("loaded ML service does not advertise its pinned space: %s", endpoint)
		}
	}
	m.mu.Lock()
	m.services.Endpoints = endpoints
	services = m.services
	m.state.Ready = true
	m.state.Stage = "ready"
	m.state.Message = "本地模型已就绪：" + m.services.Device + " " + m.state.Reason
	processes := append([]*ownedProcess(nil), m.processes...)
	m.mu.Unlock()
	for _, process := range processes {
		go func(p *ownedProcess) {
			<-p.done
			m.mu.Lock()
			if !m.closed {
				m.state.Ready = false
				m.state.Stage = "failed"
				m.state.Error = "本地模型服务已退出"
			}
			m.mu.Unlock()
		}(process)
	}
	return services, nil
}

func waitReady(ctx context.Context, endpoint string, process *ownedProcess, logs *tailBuffer) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	client := &http.Client{Timeout: time.Second}
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-process.done:
			return fmt.Errorf("ML service exited: %v: %s", process.err, logs.String())
		case <-ticker.C:
		}
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/readyz", nil)
		response, err := client.Do(request)
		if err != nil {
			continue
		}
		response.Body.Close()
		if response.StatusCode == http.StatusOK {
			return nil
		}
	}
}

func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	m.state.Ready = false
	if m.cancel != nil {
		m.cancel()
	}
	processes := append([]*ownedProcess(nil), m.processes...)
	for process := range m.active {
		processes = append(processes, process)
	}
	ensureDone := m.ensureDone
	m.mu.Unlock()
	var err error
	for _, process := range processes {
		err = errors.Join(err, process.stop(ctx))
	}
	if ensureDone != nil {
		select {
		case <-ensureDone:
		case <-ctx.Done():
			err = errors.Join(err, ctx.Err())
		}
	}
	return err
}
