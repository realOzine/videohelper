package videohelper

import (
	"io/fs"
	"strings"
	"testing"
)

func TestRuntimeAssetsAreSourcesAndLocks(t *testing.T) {
	for _, name := range []string{"ml/app/hardware.py", "ml/app/cli.py", "ml/profiles/wemm-mps/uv.lock", "ml/profiles/wemm-cpu/uv.lock", "ml/profiles/wemm-cuda/uv.lock", "ml/profiles/speech-audio-cpu/uv.lock"} {
		if _, err := RuntimeAssets.ReadFile(name); err != nil {
			t.Fatal(err)
		}
	}
	if err := fs.WalkDir(RuntimeAssets, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name != "." && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "__pycache__") {
			t.Fatalf("runtime cache was embedded: %s", name)
		}
		if !entry.IsDir() && !strings.HasSuffix(name, ".py") && !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".toml") && !strings.HasSuffix(name, "/uv.lock") {
			t.Fatalf("unexpected runtime asset: %s", name)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
