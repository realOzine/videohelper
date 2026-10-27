//go:build darwin

package desktop

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAppBundleDoesNotRewrap(t *testing.T) {
	for _, path := range []string{"/Applications/VideoHelper.app/Contents/MacOS/VideoHelper", "/tmp/MyGo Dev.app/Contents/MacOS/VideoHelper"} {
		if !inAppBundle(path) {
			t.Fatalf("bundle would wrap again: %s", path)
		}
	}
	for _, path := range []string{"/tmp/VideoHelper", "/tmp/VideoHelper.app/VideoHelper", "/tmp/VideoHelper.app/Contents/VideoHelper"} {
		if inAppBundle(path) {
			t.Fatalf("unbundled path accepted: %s", path)
		}
	}
}

func TestBundleUpdateAndFailedCopyPreservesPrevious(t *testing.T) {
	source := filepath.Join(t.TempDir(), "VideoHelper")
	if err := os.WriteFile(source, []byte("version one"), 0700); err != nil {
		t.Fatal(err)
	}
	dataDir := t.TempDir()
	bundle, err := prepareBundle(source, dataDir)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(bundle, "Contents", "MacOS", "VideoHelper")
	if b, err := os.ReadFile(target); err != nil || string(b) != "version one" {
		t.Fatalf("first copy: %s %v", b, err)
	}
	if err := os.WriteFile(source, []byte("version two"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareBundle(source, dataDir); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(target); err != nil || string(b) != "version two" {
		t.Fatalf("update: %s %v", b, err)
	}
	if _, err := prepareBundle(filepath.Join(t.TempDir(), "missing"), dataDir); err == nil {
		t.Fatal("missing source accepted")
	}
	if b, err := os.ReadFile(target); err != nil || string(b) != "version two" {
		t.Fatalf("failed copy damaged previous app: %s %v", b, err)
	}
	if _, err := prepareBundle(source, dataDir); err != nil {
		t.Fatalf("same binary cannot be reused: %v", err)
	}
	icon := filepath.Join(bundle, "Contents", "Resources", "AppIcon.icns")
	if b, err := os.ReadFile(icon); err != nil || !bytes.Equal(b, bundleIcon) {
		t.Fatalf("brand icon not restored: %v", err)
	}
	if err := os.WriteFile(icon, []byte("damaged icon"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareBundle(source, dataDir); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(icon); err != nil || !bytes.Equal(b, bundleIcon) {
		t.Fatalf("unchanged binary did not repair its icon: %v", err)
	}
}

func TestBuiltStandaloneRestoresValidSignature(t *testing.T) {
	source := os.Getenv("VH_TEST_DESKTOP_EXECUTABLE")
	if source == "" {
		t.Skip("set VH_TEST_DESKTOP_EXECUTABLE to a MyGo-built standalone binary")
	}
	bundle, err := prepareBundle(source, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", bundle).CombinedOutput(); err != nil {
		t.Fatalf("restored signature invalid: %v: %s", err, output)
	}
}
