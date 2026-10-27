package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ozine/videohelper/internal/timeline"
)

type ProbeInfo struct {
	DurationMs         int
	HasVideo, HasAudio bool
	Width, Height      int
	FPS                float64
}

// Zero options select the normalization defaults: 720p, one-second GOP, CRF 23.
type TranscodeOptions struct {
	MaxHeight  int
	GOPSeconds int
	CRF        int
}

func (t Tools) Probe(ctx context.Context, src string) (ProbeInfo, error) {
	var output bytes.Buffer
	err := t.run(ctx, "ffprobe", src, &output, "-v", "error", "-show_entries",
		"format=duration:stream=codec_type,width,height,avg_frame_rate,r_frame_rate,duration", "-of", "json", "-i", src)
	if err != nil {
		return ProbeInfo{}, err
	}
	var data struct {
		Format  struct{ Duration string }
		Streams []struct {
			CodecType     string `json:"codec_type"`
			Width, Height int
			AvgFrameRate  string `json:"avg_frame_rate"`
			RFrameRate    string `json:"r_frame_rate"`
			Duration      string
		}
	}
	if err := json.Unmarshal(output.Bytes(), &data); err != nil {
		return ProbeInfo{}, fmt.Errorf("decode ffprobe output: %w", err)
	}
	seconds, _ := strconv.ParseFloat(data.Format.Duration, 64)
	var info ProbeInfo
	for _, stream := range data.Streams {
		if duration, err := strconv.ParseFloat(stream.Duration, 64); err == nil {
			seconds = max(seconds, duration)
		}
		switch stream.CodecType {
		case "video":
			if !info.HasVideo {
				info.Width, info.Height = stream.Width, stream.Height
				info.FPS = frameRate(stream.AvgFrameRate)
				if info.FPS == 0 {
					info.FPS = frameRate(stream.RFrameRate)
				}
			}
			info.HasVideo = true
		case "audio":
			info.HasAudio = true
		}
	}
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds > float64(math.MaxInt)/1000 {
		return ProbeInfo{}, errors.New("ffprobe returned an invalid duration")
	}
	info.DurationMs = int(math.Round(seconds * 1000))
	return info, nil
}

func frameRate(value string) float64 {
	numerator, denominator, ok := strings.Cut(value, "/")
	n, err := strconv.ParseFloat(numerator, 64)
	if err != nil {
		return 0
	}
	if ok {
		d, err := strconv.ParseFloat(denominator, 64)
		if err != nil || d <= 0 {
			return 0
		}
		n /= d
	}
	if n <= 0 || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	return n
}

func (t Tools) Transcode(ctx context.Context, src, dst string, o TranscodeOptions) error {
	if o.MaxHeight == 0 {
		o.MaxHeight = 720
	}
	if o.GOPSeconds == 0 {
		o.GOPSeconds = 1
	}
	if o.CRF == 0 {
		o.CRF = 23
	}
	if o.MaxHeight < 2 || o.GOPSeconds < 1 || o.CRF < 0 || o.CRF > 51 {
		return errors.New("invalid transcode options")
	}
	info, err := t.Probe(ctx, src)
	if err != nil {
		return err
	}
	if !info.HasVideo && !info.HasAudio {
		return errors.New("input has no video or audio stream")
	}
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", src,
		"-map", "0:v:0?", "-map", "0:a:0?"}
	if info.HasVideo {
		if info.FPS == 0 || info.FPS*float64(o.GOPSeconds) > float64(math.MaxInt) {
			return errors.New("cannot determine video GOP")
		}
		gop := strconv.Itoa(max(1, int(math.Round(info.FPS*float64(o.GOPSeconds)))))
		args = append(args, "-vf", fmt.Sprintf("scale=-2:trunc(min(ih\\,%d)/2)*2", o.MaxHeight),
			"-c:v", "libx264", "-preset", "veryfast", "-crf", strconv.Itoa(o.CRF), "-pix_fmt", "yuv420p",
			"-g", gop, "-keyint_min", gop, "-sc_threshold", "0", "-fps_mode", "passthrough")
	}
	args = append(args, "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart", "-f", "mp4")
	// Publish only complete output; linking a private temporary file cannot replace dst.
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".transcode-*.mp4")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := t.run(ctx, "ffmpeg", src, nil, append(args, tmp.Name())...); err != nil {
		return err
	}
	return os.Link(tmp.Name(), dst)
}

func (t Tools) Frames(ctx context.Context, src string, atMs []int, width int) ([][]byte, error) {
	if width <= 0 {
		return nil, errors.New("frame width must be positive")
	}
	for _, at := range atMs {
		if at < 0 {
			return nil, errors.New("frame timestamp must be nonnegative")
		}
	}
	frames := make([][]byte, 0, len(atMs))
	// shortcut: one ffmpeg process per frame; batch extraction when indexing throughput requires it.
	for _, at := range atMs {
		var output bytes.Buffer
		err := t.run(ctx, "ffmpeg", src, &output, "-hide_banner", "-loglevel", "error", "-nostdin",
			"-ss", fmt.Sprintf("%d.%03d", at/1000, at%1000), "-i", src, "-map", "0:v:0", "-frames:v", "1",
			"-vf", fmt.Sprintf("scale=%d:-1", width), "-f", "image2pipe", "-c:v", "mjpeg", "pipe:1")
		if err != nil {
			return nil, err
		}
		if output.Len() == 0 {
			return nil, fmt.Errorf("no video frame at %d ms", at)
		}
		frames = append(frames, output.Bytes())
	}
	return frames, nil
}

func (t Tools) Sprites(ctx context.Context, src, outDir string, cells []timeline.Span, tileW, cols, rows int) (paths []string, layout timeline.SpriteLayout, err error) {
	if tileW <= 0 || cols <= 0 || rows <= 0 || cols > math.MaxInt/rows {
		return nil, layout, errors.New("invalid sprite dimensions")
	}
	for _, cell := range cells {
		if cell.StartMs < 0 || cell.EndMs <= cell.StartMs {
			return nil, layout, errors.New("invalid sprite cell")
		}
	}
	layout = timeline.SpriteLayout{TileW: tileW, Cols: cols, Rows: rows, Count: len(cells)}
	if len(cells) == 0 {
		return nil, layout, nil
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, layout, err
	}
	defer func() {
		if err != nil {
			for _, path := range paths {
				os.Remove(path)
			}
			paths = nil
		}
	}()
	for start := 0; start < len(cells); start += cols * rows {
		end := start + min(cols*rows, len(cells)-start)
		at := make([]int, end-start)
		for i := range at {
			at[i] = cells[start+i].StartMs
		}
		frames, err := t.Frames(ctx, src, at, tileW)
		if err != nil {
			return paths, layout, err
		}
		var sheet *image.RGBA
		for i, frame := range frames {
			img, err := jpeg.Decode(bytes.NewReader(frame))
			if err != nil {
				return paths, layout, fmt.Errorf("decode sprite frame: %w", err)
			}
			if layout.TileH == 0 {
				layout.TileH = img.Bounds().Dy()
			}
			if img.Bounds().Dx() != tileW || img.Bounds().Dy() != layout.TileH || tileW > math.MaxInt/cols || layout.TileH > math.MaxInt/rows {
				return paths, layout, errors.New("inconsistent or oversized sprite frame")
			}
			if sheet == nil {
				sheet = image.NewRGBA(image.Rect(0, 0, tileW*cols, layout.TileH*rows))
			}
			x, y := i%cols*tileW, i/cols*layout.TileH
			draw.Draw(sheet, image.Rect(x, y, x+tileW, y+layout.TileH), img, img.Bounds().Min, draw.Src)
		}
		path := filepath.Join(outDir, strconv.Itoa(len(paths))+".jpg")
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return paths, layout, err
		}
		paths = append(paths, path)
		encodeErr := jpeg.Encode(file, sheet, &jpeg.Options{Quality: 85})
		closeErr := file.Close()
		if err := errors.Join(encodeErr, closeErr); err != nil {
			return paths, layout, err
		}
	}
	return paths, layout, nil
}

const stderrLimit = 4096

type boundedStderr struct {
	data      []byte
	truncated bool
}

func (b *boundedStderr) Write(p []byte) (int, error) {
	n := len(p)
	if len(b.data)+n > stderrLimit {
		b.truncated = true
	}
	if n >= stderrLimit {
		b.data = append(b.data[:0], p[n-stderrLimit:]...)
	} else {
		if extra := len(b.data) + n - stderrLimit; extra > 0 {
			b.data = b.data[extra:]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}

var loggedURL = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s'"<>]+`)

func (b *boundedStderr) message(src string) string {
	data := b.data
	if b.truncated {
		// Drop a cut-off URL or credential at the beginning of the retained tail.
		if newline := bytes.IndexByte(data, '\n'); newline >= 0 {
			data = data[newline+1:]
		} else {
			return ""
		}
	}
	message := string(data)
	if src != "" {
		message = strings.ReplaceAll(message, src, "[input]")
	}
	if u, err := url.Parse(src); err == nil {
		for _, values := range u.Query() {
			for _, value := range values {
				if value != "" {
					message = strings.ReplaceAll(message, value, "[redacted]")
					message = strings.ReplaceAll(message, url.QueryEscape(value), "[redacted]")
				}
			}
		}
		if u.User != nil {
			if username := u.User.Username(); username != "" {
				message = strings.ReplaceAll(message, username, "[redacted]")
			}
			if password, ok := u.User.Password(); ok && password != "" {
				message = strings.ReplaceAll(message, password, "[redacted]")
			}
		}
	}
	message = strings.TrimSpace(loggedURL.ReplaceAllString(message, "[URL]"))
	if len(message) > stderrLimit {
		message = message[:stderrLimit]
	}
	return message
}

func (t Tools) run(ctx context.Context, name, src string, stdout io.Writer, args ...string) error {
	executable := name
	if name == "ffmpeg" && t.FFmpeg != "" {
		executable = t.FFmpeg
	}
	if name == "ffprobe" && t.FFprobe != "" {
		executable = t.FFprobe
	}
	var stderr boundedStderr
	execute := t.RunCommand
	if execute == nil {
		execute = func(ctx context.Context, path string, args []string, stdout, stderr io.Writer) error {
			cmd := exec.CommandContext(ctx, path, args...)
			cmd.Stdout, cmd.Stderr = stdout, stderr
			return cmd.Run()
		}
	}
	if err := execute(ctx, executable, args, stdout, &stderr); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%s: %w", name, ctx.Err())
		}
		return fmt.Errorf("%s: %w: %s", name, err, stderr.message(src))
	}
	return nil
}
