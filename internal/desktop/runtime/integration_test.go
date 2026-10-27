package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestYTDLPPrivateMediaToolsIntegration(t *testing.T) {
	dir := os.Getenv("VH_TEST_DESKTOP_TOOLS_DIR")
	if dir == "" {
		t.Skip("set VH_TEST_DESKTOP_TOOLS_DIR to a disposable private directory")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	components, err := componentsFor(runtime.GOOS + "/" + runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, component := range components {
		if component.Notice || component.Name == "uv" {
			continue
		}
		paths[component.Name], err = installComponent(ctx, downloadClient(), filepath.Join(dir, "runtime", "tools"), component)
		if err != nil {
			t.Fatal(err)
		}
	}
	ffmpeg, ffprobe, err := mediaToolset(filepath.Join(dir, "runtime", "tools"), paths["ffmpeg"], paths["ffprobe"])
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(ffmpeg) != filepath.Dir(ffprobe) {
		t.Fatal("private FFmpeg tools are not neighbors")
	}
	t.Setenv("PATH", "")
	fixture := t.TempDir()
	var logs tailBuffer
	run := func(path string, args ...string) {
		t.Helper()
		if err := RunCommand(ctx, path, args, &logs, &logs); err != nil {
			t.Fatalf("%s: %v\n%s", filepath.Base(path), err, logs.String())
		}
	}
	run(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=blue:s=32x32:r=10", "-t", "1", "-an", "-c:v", "libx264", filepath.Join(fixture, "video.mp4"))
	run(ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "1", "-vn", "-c:a", "aac", filepath.Join(fixture, "audio.m4a"))
	manifest := `<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" type="static" mediaPresentationDuration="PT1S" minBufferTime="PT1S"><Period duration="PT1S"><AdaptationSet mimeType="video/mp4" contentType="video" codecs="avc1.64000a" width="32" height="32" frameRate="10"><Representation id="video" bandwidth="100000"><BaseURL>video.mp4</BaseURL></Representation></AdaptationSet><AdaptationSet mimeType="audio/mp4" contentType="audio" codecs="mp4a.40.2"><Representation id="audio" bandwidth="64000" audioSamplingRate="48000"><BaseURL>audio.m4a</BaseURL></Representation></AdaptationSet></Period></MPD>`
	if err := os.WriteFile(filepath.Join(fixture, "manifest.mpd"), []byte(manifest), 0600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.mpd" {
			w.Header().Set("Content-Type", "application/dash+xml")
		}
		http.ServeFile(w, r, filepath.Join(fixture, filepath.Base(r.URL.Path)))
	}))
	defer server.Close()
	common := []string{"--ignore-config", "--no-plugin-dirs", "--no-playlist", "--no-update", "--ffmpeg-location", filepath.Dir(ffmpeg), "--js-runtimes", "deno:" + paths["deno"]}
	run(paths["yt-dlp"], append(common, "--format", "bestvideo+bestaudio", "--merge-output-format", "mp4", "--output", filepath.Join(fixture, "merged.%(ext)s"), "--", server.URL+"/manifest.mpd")...)
	var probe tailBuffer
	if err := RunCommand(ctx, ffprobe, []string{"-v", "error", "-show_entries", "stream=codec_type", "-of", "json", filepath.Join(fixture, "merged.mp4")}, &probe, &logs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(probe.String(), "video") || !strings.Contains(probe.String(), "audio") {
		t.Fatalf("merged media lacks streams: %s", probe.String())
	}
	run(paths["yt-dlp"], append(common, "--format", "bestaudio", "--extract-audio", "--audio-format", "wav", "--output", filepath.Join(fixture, "extracted.%(ext)s"), "--", server.URL+"/manifest.mpd")...)
	if _, err := os.Stat(filepath.Join(fixture, "extracted.wav")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "[Merger]") || !strings.Contains(logs.String(), "[ExtractAudio]") {
		t.Fatalf("postprocessors did not run: %s", logs.String())
	}
	if err := os.WriteFile(filepath.Join(dir, "yt-dlp-private-tools.log"), []byte(logs.String()+"\nMerged streams:\n"+probe.String()), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log("yt-dlp downloaded separate DASH tracks, merged video+audio and probed/extracted WAV with empty PATH", filepath.Dir(ffmpeg))
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
	for _, profile := range []string{m.visualProfile, "speech-audio-cpu"} {
		if _, err := m.run(ctx, m.profilePython(profile), []string{"-c", "from importlib.util import find_spec; assert all(find_spec(name) is None for name in ('boto3','botocore','s3transfer'))"}, nil); err != nil {
			t.Fatal(err)
		}
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
