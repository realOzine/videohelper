package media

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestPrivateToolsUseSuppliedRunner(t *testing.T) {
	called := false
	tools := Tools{FFprobe: "/private/tools/ffprobe", RunCommand: func(ctx context.Context, path string, args []string, stdout, stderr io.Writer) error {
		called = true
		if path != "/private/tools/ffprobe" || args[len(args)-1] != "/library/source.mp4" {
			t.Fatalf("wrong private command %q %q", path, args)
		}
		_, err := io.Copy(stdout, strings.NewReader(`{"format":{"duration":"6"},"streams":[{"codec_type":"video","width":320,"height":180,"avg_frame_rate":"24/1"}]}`))
		return err
	}}
	info, err := tools.Probe(context.Background(), "/library/source.mp4")
	if err != nil || !called || info.DurationMs != 6000 || !info.HasVideo || info.FPS != 24 {
		t.Fatalf("%+v %v", info, err)
	}
}
