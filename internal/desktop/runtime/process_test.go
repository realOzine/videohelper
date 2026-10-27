package runtime

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestOwnedProcessCancelsDescendants(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "child")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- RunCommand(ctx, executable, []string{"-test.run=^TestRuntimeProcessHelper$", "--", "parent", marker}, nil, nil)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("process tree did not stop")
	}
	before, _ := os.Stat(marker)
	time.Sleep(200 * time.Millisecond)
	after, _ := os.Stat(marker)
	if before.ModTime() != after.ModTime() {
		t.Fatal("descendant survived cancellation")
	}
}

func TestRuntimeProcessHelper(t *testing.T) {
	if len(os.Args) < 4 || os.Args[len(os.Args)-3] != "--" {
		return
	}
	mode, marker := os.Args[len(os.Args)-2], os.Args[len(os.Args)-1]
	if mode == "parent" {
		executable, _ := os.Executable()
		cmd := exec.Command(executable, "-test.run=^TestRuntimeProcessHelper$", "--", "child", marker)
		if err := cmd.Start(); err != nil {
			os.Exit(2)
		}
	} else {
		for {
			if err := os.WriteFile(marker, []byte("alive"), 0600); err != nil {
				os.Exit(2)
			}
			time.Sleep(25 * time.Millisecond)
		}
	}
	for {
		time.Sleep(time.Second)
	}
}
