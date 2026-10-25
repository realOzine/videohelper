package blob

import (
	"strings"
	"testing"
)

func TestRunArtifactsAreIsolatedAndPlaybackUsesPublishedRun(t *testing.T) {
	key := RunNormalizedVideoKey(1, 2, "v1.generation", 3)
	if key != "workspaces/1/videos/2/runs/v1.generation/3/normalized_video.mp4" {
		t.Fatal(key)
	}
	if key == RunNormalizedVideoKey(1, 2, "v1.generation", 4) || key == RunNormalizedVideoKey(1, 2, "v1.restored", 3) {
		t.Fatal("attempts can overwrite published media")
	}
	if SpriteKeyForVideo(key, 7) != RunSpriteKey(1, 2, "v1.generation", 3, 7) {
		t.Fatal("thumbnail points outside published run")
	}
	if !strings.HasPrefix(RunRawKey(1, 2, "v1.generation", 3, "asr/chunk-0.json"), VideoPrefix(1, 2)) {
		t.Fatal("raw artifact escapes video cleanup prefix")
	}
}
