package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ozine/videohelper/internal/timeline"
)

func requireFFmpeg(t *testing.T) {
	t.Helper()
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("%s is not installed", name)
		}
	}
}

func fixture(t *testing.T, audio bool) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.mp4")
	args := []string{"-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i",
		"color=c=red:s=960x800:r=10:d=1[r];color=c=green:s=960x800:r=10:d=1[g];color=c=blue:s=960x800:r=10:d=1[b];[r][g][b]concat=n=3:v=1:a=0"}
	if audio {
		args = append(args, "-itsoffset", "0.4", "-f", "lavfi", "-i", "sine=frequency=440:duration=2.6", "-c:a", "aac")
	}
	args = append(args, "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", path)
	if output, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("create synthetic fixture: %v: %s", err, output)
	}
	return path
}

type inspectedMedia struct {
	Streams []struct {
		CodecName     string `json:"codec_name"`
		CodecType     string `json:"codec_type"`
		StartTime     string `json:"start_time"`
		Width, Height int
	}
	Frames []struct {
		Time string `json:"best_effort_timestamp_time"`
	}
}

func inspect(t *testing.T, path string, keyframes bool) inspectedMedia {
	t.Helper()
	args := []string{"-v", "error", "-show_entries", "stream=codec_name,codec_type,width,height,start_time", "-of", "json"}
	if keyframes {
		args = append(args, "-select_streams", "v:0", "-skip_frame", "nokey", "-show_frames", "-show_entries", "frame=best_effort_timestamp_time")
	}
	output, err := exec.Command("ffprobe", append(args, path)...).Output()
	if err != nil {
		t.Fatal(err)
	}
	var got inspectedMedia
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestProbeAndTranscode(t *testing.T) {
	requireFFmpeg(t)
	ctx := context.Background()
	src := fixture(t, true)
	info, err := Probe(ctx, src)
	if err != nil || !info.HasVideo || !info.HasAudio || info.Width != 960 || info.Height != 800 || info.FPS != 10 || math.Abs(float64(info.DurationMs-3000)) > 50 {
		t.Fatalf("Probe: %+v, %v", info, err)
	}
	for _, tt := range []struct {
		name          string
		options       TranscodeOptions
		width, height int
		keys          []string
	}{
		{"defaults", TranscodeOptions{}, 864, 720, []string{"0.000000", "1.000000", "2.000000"}},
		{"options", TranscodeOptions{MaxHeight: 360, GOPSeconds: 2, CRF: 28}, 432, 360, []string{"0.000000", "2.000000"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dst := filepath.Join(t.TempDir(), "normalized.mp4")
			if err := Transcode(ctx, src, dst, tt.options); err != nil {
				t.Fatal(err)
			}
			out, err := Probe(ctx, dst)
			if err != nil || out.Width != tt.width || out.Height != tt.height || !out.HasAudio || math.Abs(float64(out.DurationMs-info.DurationMs)) > 100 {
				t.Fatalf("normalized Probe: %+v, %v", out, err)
			}
			streams := inspect(t, dst, false).Streams
			if len(streams) != 2 || streams[0].CodecName != "h264" || streams[1].CodecName != "aac" {
				t.Fatalf("unexpected codecs: %+v", streams)
			}
			// Delayed audio must retain its offset relative to video after AAC priming.
			if streams[0].StartTime != "0.000000" || streams[1].StartTime < "0.300000" || streams[1].StartTime > "0.450000" {
				t.Fatalf("A/V start offsets changed: %+v", streams)
			}
			frames := inspect(t, dst, true).Frames
			if len(frames) != len(tt.keys) {
				t.Fatalf("keyframes: %+v, want %v", frames, tt.keys)
			}
			for i, frame := range frames {
				if frame.Time != tt.keys[i] {
					t.Fatalf("keyframe %d at %s, want %s", i, frame.Time, tt.keys[i])
				}
			}
			data, err := os.ReadFile(dst)
			if err != nil || bytes.Index(data, []byte("moov")) < 0 || bytes.Index(data, []byte("moov")) > bytes.Index(data, []byte("mdat")) {
				t.Fatalf("faststart atoms missing/out of order: %v", err)
			}
		})
	}
	t.Run("no audio", func(t *testing.T) {
		dst := filepath.Join(t.TempDir(), "silent.mp4")
		if err := Transcode(ctx, fixture(t, false), dst, TranscodeOptions{MaxHeight: 120}); err != nil {
			t.Fatal(err)
		}
		if info, err := Probe(ctx, dst); err != nil || info.HasAudio || !info.HasVideo {
			t.Fatalf("silent output: %+v, %v", info, err)
		}
	})
	t.Run("existing destination", func(t *testing.T) {
		dir := t.TempDir()
		dst := filepath.Join(dir, "existing.mp4")
		if err := os.WriteFile(dst, []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := Transcode(ctx, src, dst, TranscodeOptions{MaxHeight: 120}); err == nil {
			t.Fatal("overwrote existing destination")
		}
		if data, err := os.ReadFile(dst); err != nil || string(data) != "keep" {
			t.Fatalf("destination changed: %q, %v", data, err)
		}
		if files, _ := os.ReadDir(dir); len(files) != 1 {
			t.Fatalf("temporary output was retained: %v", files)
		}
	})
}

func jpegImage(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func assertColor(t *testing.T, img image.Image, x, y int, want string) {
	t.Helper()
	r, g, b, _ := img.At(x, y).RGBA()
	valid := map[string]bool{
		"red":   r > 20000 && r > 2*g && r > 2*b,
		"green": g > 10000 && g > 2*r && g > 2*b,
		"blue":  b > 20000 && b > 2*r && b > 2*g,
		"black": r < 2000 && g < 2000 && b < 2000,
	}[want]
	if !valid {
		t.Fatalf("pixel (%d,%d) = (%d,%d,%d), want %s", x, y, r, g, b, want)
	}
}

func TestFramesAndSprites(t *testing.T) {
	requireFFmpeg(t)
	src := fixture(t, false)
	ctx := context.Background()
	frames, err := Frames(ctx, src, []int{2250, 0, 1250}, 96)
	if err != nil || len(frames) != 3 {
		t.Fatalf("Frames: count=%d, %v", len(frames), err)
	}
	for i, color := range []string{"blue", "red", "green"} {
		img := jpegImage(t, frames[i])
		if img.Bounds().Dx() != 96 || img.Bounds().Dy() != 80 {
			t.Fatalf("frame size: %v", img.Bounds())
		}
		assertColor(t, img, 48, 40, color)
	}
	cells := []timeline.Span{
		{StartMs: 0, EndMs: 1000}, {StartMs: 1250, EndMs: 1500}, {StartMs: 2250, EndMs: 2500},
		{StartMs: 500, EndMs: 700}, {StartMs: 1750, EndMs: 1900},
	}
	paths, layout, err := Sprites(ctx, src, filepath.Join(t.TempDir(), "sprites"), cells, 96, 2, 2)
	if err != nil || len(paths) != 2 || layout != (timeline.SpriteLayout{TileW: 96, TileH: 80, Cols: 2, Rows: 2, Count: 5}) {
		t.Fatalf("Sprites: paths=%v layout=%+v err=%v", paths, layout, err)
	}
	for sprite, colors := range [][]string{{"red", "green", "blue", "red"}, {"green", "black", "black", "black"}} {
		data, err := os.ReadFile(paths[sprite])
		if err != nil {
			t.Fatal(err)
		}
		img := jpegImage(t, data)
		info := inspect(t, paths[sprite], false)
		if len(info.Streams) != 1 || info.Streams[0].CodecName != "mjpeg" || info.Streams[0].Width != 192 || info.Streams[0].Height != 160 {
			t.Fatalf("sprite ffprobe: %+v", info)
		}
		for cell, color := range colors {
			assertColor(t, img, cell%2*96+48, cell/2*80+40, color)
		}
	}
	if _, err := Frames(ctx, src, []int{99999}, 96); err == nil {
		t.Fatal("accepted a frame beyond end of video")
	}
	t.Run("failed sprite creation removes only its own outputs", func(t *testing.T) {
		dir := t.TempDir()
		existing := filepath.Join(dir, "1.jpg")
		if err := os.WriteFile(existing, []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := Sprites(ctx, src, dir, cells, 96, 2, 2); err == nil {
			t.Fatal("replaced an existing sprite")
		}
		if files, _ := os.ReadDir(dir); len(files) != 1 || files[0].Name() != "1.jpg" {
			t.Fatalf("partial outputs were not removed: %v", files)
		}
		if data, err := os.ReadFile(existing); err != nil || string(data) != "keep" {
			t.Fatalf("existing sprite changed: %q, %v", data, err)
		}
	})
}

func TestCommandCancellationAndErrors(t *testing.T) {
	requireFFmpeg(t)
	t.Run("running ffmpeg cancellation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()
		start := time.Now()
		err := run(ctx, "ffmpeg", "", nil, "-hide_banner", "-loglevel", "error", "-nostdin", "-re", "-f", "lavfi", "-i", "color=s=16x16:r=10", "-t", "30", "-f", "null", "-")
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 3*time.Second {
			t.Fatalf("ffmpeg did not cancel promptly: %v (%s)", err, time.Since(start))
		}
	})
	t.Run("running command cancellation", func(t *testing.T) {
		started, release := make(chan struct{}, 1), make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
		}))
		defer server.Close()
		defer close(release)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := make(chan error, 1)
		go func() { _, err := Probe(ctx, server.URL+"/blocked.mp4"); result <- err }()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("ffprobe did not request input")
		}
		cancel()
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation error: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("canceled ffprobe is still running")
		}
	})
	t.Run("presigned URL redaction", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
		defer server.Close()
		src := server.URL + "/private.mp4?X-Amz-Signature=secret-signature&token=secret-token"
		_, err := Probe(context.Background(), src)
		if err == nil || strings.Contains(err.Error(), "secret-") || strings.Contains(err.Error(), "X-Amz-Signature") || !strings.Contains(err.Error(), "403") {
			t.Fatalf("unsafe or unhelpful error: %v", err)
		}
		_, err = Frames(context.Background(), src, []int{0}, 96)
		if err == nil || strings.Contains(err.Error(), "secret-") || strings.Contains(err.Error(), "X-Amz-Signature") || !strings.Contains(err.Error(), "403") {
			t.Fatalf("unsafe or unhelpful ffmpeg error: %v", err)
		}
	})
	t.Run("bounded stderr", func(t *testing.T) {
		var stderr boundedStderr
		stderr.Write([]byte(strings.Repeat("secret", 1000)))
		stderr.Write([]byte("\nfailed https://example.com/private?token=secret-token\nreason: secret-token denied\n"))
		message := stderr.message("https://example.com/private?token=secret-token")
		if len(stderr.data) > stderrLimit || len(message) > stderrLimit || strings.Contains(message, "secret") || !strings.Contains(message, "denied") {
			t.Fatalf("unsafe or unbounded message: %q", message)
		}
	})
}

func TestInvalidMediaOptions(t *testing.T) {
	ctx := context.Background()
	if err := Transcode(ctx, "unused", "unused", TranscodeOptions{MaxHeight: -1}); err == nil {
		t.Fatal("accepted negative height")
	}
	if _, err := Frames(ctx, "unused", []int{-1}, 96); err == nil {
		t.Fatal("accepted negative frame time")
	}
	if _, err := Frames(ctx, "unused", nil, 0); err == nil {
		t.Fatal("accepted zero width")
	}
	if _, _, err := Sprites(ctx, "unused", t.TempDir(), []timeline.Span{{StartMs: 0, EndMs: 1}}, 96, 0, 2); err == nil {
		t.Fatal("accepted zero sprite columns")
	}
	if _, _, err := Sprites(ctx, "unused", t.TempDir(), []timeline.Span{{StartMs: 1, EndMs: 0}}, 96, 2, 2); err == nil {
		t.Fatal("accepted invalid cell interval")
	}
}
