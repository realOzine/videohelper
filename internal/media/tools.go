package media

import (
	"context"
	"io"

	"github.com/ozine/videohelper/internal/timeline"
)

// Tools selects private executables and their lifecycle without changing PATH.
// Zero values retain command-line/server behavior.
type Tools struct {
	FFmpeg, FFprobe string
	RunCommand      func(context.Context, string, []string, io.Writer, io.Writer) error
}

func Probe(ctx context.Context, src string) (ProbeInfo, error) { return (Tools{}).Probe(ctx, src) }
func Transcode(ctx context.Context, src, dst string, o TranscodeOptions) error {
	return (Tools{}).Transcode(ctx, src, dst, o)
}
func Frames(ctx context.Context, src string, atMs []int, width int) ([][]byte, error) {
	return (Tools{}).Frames(ctx, src, atMs, width)
}
func Sprites(ctx context.Context, src, outDir string, cells []timeline.Span, tileW, cols, rows int) ([]string, timeline.SpriteLayout, error) {
	return (Tools{}).Sprites(ctx, src, outDir, cells, tileW, cols, rows)
}
func run(ctx context.Context, name, src string, stdout io.Writer, args ...string) error {
	return (Tools{}).run(ctx, name, src, stdout, args...)
}
