package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ozine/videohelper/internal/desktop/filelock"
)

func TestPackagedAssetsAreVersionedAndReused(t *testing.T) {
	assets := fstest.MapFS{"ml/pyproject.toml": {Data: []byte("project")}, "ml/app/hardware.py": {Data: []byte("hardware")}}
	dir := t.TempDir()
	first, err := extractAssets(assets, dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := extractAssets(assets, dir)
	if err != nil || first != second {
		t.Fatalf("reuse: %s %v", second, err)
	}
	assets["ml/app/hardware.py"].Data = []byte("new hardware")
	third, err := extractAssets(assets, dir)
	if err != nil || first == third {
		t.Fatalf("version: %s %v", third, err)
	}
	if _, err := os.Stat(filepath.Join(first, "ml", "app", "hardware.py")); err != nil {
		t.Fatal("old runtime removed")
	}
	if _, err := extractAssets(nil, dir); err == nil {
		t.Fatal("accepted missing packaged assets")
	}
}

func TestEnvironmentCannotEnableFakeModelsOrUseCallerPython(t *testing.T) {
	t.Setenv("ML_FAKE", "1")
	t.Setenv("ML_S3_ENDPOINT", "http://unexpected")
	t.Setenv("PYTHONPATH", "untrusted")
	t.Setenv("VIRTUAL_ENV", "untrusted")
	t.Setenv("UV_PROJECT_ENVIRONMENT", "untrusted")
	t.Setenv("HF_HOME", "untrusted")
	m := New(t.TempDir(), nil)
	env := make(map[string]string)
	for _, item := range m.environment(nil) {
		key, value, _ := strings.Cut(item, "=")
		env[key] = value
	}
	if env["ML_FAKE"] != "0" || env["PYTHONPATH"] != "" || env["VIRTUAL_ENV"] != "" || env["UV_PROJECT_ENVIRONMENT"] != "" || env["ML_S3_ENDPOINT"] != "" {
		t.Fatalf("unsafe environment: %v", env)
	}
	if !strings.HasPrefix(env["HF_HOME"], m.dir) || !strings.HasPrefix(env["UV_PYTHON_INSTALL_DIR"], m.dir) {
		t.Fatal("runtime escaped private directory")
	}
	if m.State().Ready {
		t.Fatal("new manager is prematurely ready")
	}
}

func TestCloseCancelsBootstrapWaitingForAnotherInstaller(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "runtime"), 0700)
	lock, err := filelock.Try(filepath.Join(dir, "runtime", "install.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	m := New(dir, nil)
	done := make(chan error, 1)
	go func() { done <- m.Ensure(context.Background(), nil) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		m.mu.Lock()
		started := m.cancel != nil
		m.mu.Unlock()
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Ensure did not start")
		}
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := m.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("bootstrap cancel: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("bootstrap survived Close")
	}
	if m.State().Ready {
		t.Fatal("closed manager is ready")
	}
}

func TestCloseWaitsForActiveBootstrapProcessTree(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "child")
	m := New(dir, nil)
	m.root = dir
	done := make(chan error, 1)
	go func() {
		_, err := m.run(context.Background(), executable, []string{"-test.run=^TestRuntimeProcessHelper$", "--", "parent", marker}, nil)
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("bootstrap child did not start")
		}
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := m.Close(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("bootstrap not reaped by Close")
	}
	before, _ := os.Stat(marker)
	time.Sleep(200 * time.Millisecond)
	after, _ := os.Stat(marker)
	if before.ModTime() != after.ModTime() {
		t.Fatal("bootstrap descendant survived Close")
	}
}
