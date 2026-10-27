package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	assets "github.com/ozine/videohelper"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/mlclient"
)

// Opt-in: downloads pinned tools/Python, retains the private cache, and never loads model weights.
func TestPinnedToolsIntegration(t *testing.T) {
	dir := os.Getenv("VH_TEST_DESKTOP_TOOLS_DIR")
	if dir == "" {
		t.Skip("set VH_TEST_DESKTOP_TOOLS_DIR to a disposable private directory")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	components, err := componentsFor(runtime.GOOS + "/" + runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	m := New(dir, nil)
	m.root = dir
	for _, component := range components {
		t.Log("verify", component.Name, component.SHA256)
		path, err := installComponent(ctx, downloadClient(), filepath.Join(dir, "runtime", "tools"), component)
		if err != nil {
			t.Fatal(err)
		}
		if component.Notice {
			continue
		}
		args := []string{"--version"}
		if component.Name == "ffmpeg" || component.Name == "ffprobe" {
			args = []string{"-version"}
		}
		out, err := m.run(ctx, path, args, nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "--enable-nonfree") {
			t.Fatal("FFmpeg component enables nonfree code")
		}
		t.Log(strings.Split(out, "\n")[0])
		if component.Name == "ffmpeg" {
			license, err := m.run(ctx, path, []string{"-L"}, nil)
			if err != nil || strings.Contains(license, "not legally redistributable") {
				t.Fatalf("FFmpeg license: %s %v", license, err)
			}
			t.Log(strings.TrimSpace(license))
		}
		if component.Name == "uv" {
			m.uv = path
		}
	}
	metadata := filepath.Join(dir, "runtime", "python-downloads.json")
	if err := os.WriteFile(metadata, pythonDownloads, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.run(ctx, m.uv, []string{"python", "install", "cpython-3.12.15", "--no-bin", "--no-registry", "--python-downloads-json-url", metadata}, nil); err != nil {
		t.Fatal(err)
	}
	python, err := m.run(ctx, m.uv, []string{"python", "find", "cpython-3.12.15", "--managed-python", "--no-python-downloads"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	version, err := m.run(ctx, strings.TrimSpace(python), []string{"--version"}, nil)
	if err != nil || !strings.Contains(version, "3.12.15") {
		t.Fatalf("private Python: %q %v", version, err)
	}
	t.Log(strings.TrimSpace(version))
}

// Opt-in real installation, model loading and text inference; diagnostic output never contains tokens.
func TestManagerIntegration(t *testing.T) {
	dir := os.Getenv("VH_TEST_DESKTOP_DIR")
	if dir == "" {
		t.Skip("set VH_TEST_DESKTOP_DIR to a disposable private directory")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	m := New(dir, assets.RuntimeAssets)
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := m.Close(closeCtx); err != nil {
			t.Error(err)
		}
	}()
	if err := m.Ensure(ctx, func(p Progress) { t.Log(p.Stage, p.Message) }); err != nil {
		t.Fatal(err)
	}
	services, err := m.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := map[string]any{"observed_at": time.Now().UTC().Format(time.RFC3339), "device": services.Device, "state": m.State()}
	for i, endpoint := range services.Endpoints {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/v1/runtime", nil)
		request.Header.Set("X-Internal-Token", services.Token)
		request.Header.Set("X-Request-ID", "desktop-runtime-integration")
		response, err := (&http.Client{Timeout: 30 * time.Second}).Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var snapshot map[string]any
		decodeErr := json.NewDecoder(response.Body).Decode(&snapshot)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || decodeErr != nil || snapshot["fake"] != false {
			t.Fatalf("runtime: %d %v %v", response.StatusCode, decodeErr, snapshot)
		}
		name := "visual"
		if i == 1 {
			name = "speech_audio"
		}
		diagnostics[name] = snapshot
	}
	client := mlclient.New(config.MLConfig{Endpoints: services.Endpoints, Token: services.Token, QueryTimeout: 2 * time.Minute}, func(id string) (string, bool) {
		switch id {
		case services.VisualSpace:
			return services.Endpoints[0], true
		case services.AudioSpace:
			return services.Endpoints[1], true
		}
		return "", false
	})
	for _, id := range []string{services.VisualSpace, services.AudioSpace} {
		response, err := client.Embed(ctx, mlclient.EmbedRequest{Space: id, Role: "query", Inputs: []mlclient.EmbedInput{{Text: "雨声"}}})
		if err != nil {
			t.Fatal(err)
		}
		if len(response.Vectors) != 1 || len(response.Vectors[0]) != response.Dim {
			t.Fatalf("invalid real query: %+v", response)
		}
		diagnostics[id] = map[string]any{"dim": response.Dim, "fingerprint": response.Fingerprint, "query": "雨声"}
		t.Log("real query", id, response.Dim, response.Fingerprint)
	}
	data, err := json.MarshalIndent(diagnostics, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.dir, "runtime-diagnostics.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("diagnostics", filepath.Join(m.dir, "runtime-diagnostics.json"))
}
