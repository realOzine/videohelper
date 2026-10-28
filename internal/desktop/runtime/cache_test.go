package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// Run the test executable as uv/Python to exercise real command cancellation without downloads.
func TestMain(m *testing.M) {
	if root := os.Getenv("VH_TEST_RUNTIME_CACHE_HELPER"); root != "" {
		os.Exit(cacheCommand(root))
	}
	os.Exit(m.Run())
}

type cacheCall struct {
	Args    []string
	Offline string
}

func cacheCommand(root string) int {
	args := os.Args[1:]
	f, err := os.OpenFile(filepath.Join(root, "calls"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return 2
	}
	json.NewEncoder(f).Encode(cacheCall{Args: args, Offline: os.Getenv("HF_HUB_OFFLINE")})
	f.Close()
	kind, check := "models", os.Getenv("HF_HUB_OFFLINE") == "1"
	if args[0] == "python" {
		kind = "python"
		check = args[1] == "find"
	}
	if args[0] == "sync" {
		kind = "profile"
		check = slices.Contains(args, "--check")
		if check && !slices.Contains(args, "--offline") {
			return 2
		}
	}
	if block, _ := os.ReadFile(filepath.Join(root, "block")); check && string(block) == kind {
		os.WriteFile(filepath.Join(root, "entered"), nil, 0600)
		for {
			time.Sleep(time.Second)
		}
	}
	flag := filepath.Join(root, kind+".present")
	if check {
		if _, err := os.Stat(flag); err != nil {
			fmt.Fprintln(os.Stderr, kind+" cache missing")
			return 1
		}
	} else {
		if _, err := os.Stat(filepath.Join(root, "fail-online")); kind == "models" && err == nil {
			fmt.Fprintln(os.Stderr, "online preparation failed")
			return 1
		}
		if err := os.WriteFile(flag, nil, 0600); err != nil {
			return 2
		}
	}
	if kind == "python" && check {
		fmt.Println(filepath.Join(root, "private-python"))
	}
	return 0
}

func cacheFixture(t *testing.T) (*Manager, func() []cacheCall) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("VH_TEST_RUNTIME_CACHE_HELPER", dir)
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	m := New(dir, nil)
	m.root = dir
	m.uv = program
	m.python = program
	python := m.profilePython("test")
	if err := os.MkdirAll(filepath.Dir(python), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(program, python); err != nil {
		t.Fatal(err)
	}
	return m, func() []cacheCall {
		f, err := os.Open(filepath.Join(dir, "calls"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		decoder := json.NewDecoder(f)
		calls := []cacheCall{}
		for {
			var call cacheCall
			err := decoder.Decode(&call)
			if err == io.EOF {
				return calls
			}
			if err != nil {
				t.Fatal(err)
			}
			calls = append(calls, call)
		}
	}
}

func TestWarmRuntimeChecksNeverEnterInstallationOrOnlinePreparation(t *testing.T) {
	m, calls := cacheFixture(t)
	for _, kind := range []string{"python", "profile", "models"} {
		if err := os.WriteFile(filepath.Join(m.dir, kind+".present"), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	stages := []string{}
	notify := func(p Progress) { stages = append(stages, p.Stage) }
	ctx := context.Background()
	if err := m.ensurePython(ctx, notify); err != nil {
		t.Fatal(err)
	}
	if err := m.ensureProfile(ctx, "test", notify); err != nil {
		t.Fatal(err)
	}
	if err := m.prepareModels(ctx, "test", "test", notify); err != nil {
		t.Fatal(err)
	}
	got := calls()
	if len(got) != 3 || got[0].Args[1] != "find" || !slices.Contains(got[0].Args, "--no-python-downloads") || !slices.Contains(got[1].Args, "--check") || !slices.Contains(got[1].Args, "--offline") || got[2].Offline != "1" {
		t.Fatalf("unexpected warm commands: %+v", got)
	}
	if !slices.Equal(stages, []string{"checking", "checking", "checking_models"}) {
		t.Fatalf("warm startup claimed installation/download: %v", stages)
	}
}

func TestMissingRuntimeInstallsOnlyAfterFailedOfflineChecks(t *testing.T) {
	m, calls := cacheFixture(t)
	stages := []string{}
	notify := func(p Progress) { stages = append(stages, p.Stage) }
	ctx := context.Background()
	if err := m.ensurePython(ctx, notify); err != nil {
		t.Fatal(err)
	}
	if err := m.ensureProfile(ctx, "test", notify); err != nil {
		t.Fatal(err)
	}
	if err := m.prepareModels(ctx, "test", "test", notify); err != nil {
		t.Fatal(err)
	}
	got := calls()
	if len(got) != 7 || got[0].Args[1] != "find" || got[1].Args[1] != "install" || got[2].Args[1] != "find" || !slices.Contains(got[3].Args, "--check") || slices.Contains(got[4].Args, "--check") || got[5].Offline != "1" || got[6].Offline != "0" {
		t.Fatalf("unexpected repair commands: %+v", got)
	}
	if !slices.Equal(stages, []string{"checking", "python", "checking", "packages", "checking_models", "models"}) {
		t.Fatalf("unexpected repair stages: %v", stages)
	}
}

func TestModelPreparationPreservesOfflineFailure(t *testing.T) {
	m, _ := cacheFixture(t)
	if err := os.WriteFile(filepath.Join(m.dir, "fail-online"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	err := m.prepareModels(context.Background(), "test", "test", nil)
	if err == nil || !strings.Contains(err.Error(), "models cache missing") || !strings.Contains(err.Error(), "online preparation failed") {
		t.Fatalf("lost cache diagnosis: %v", err)
	}
}

func TestCancelledCacheChecksNeverTriggerRepair(t *testing.T) {
	for _, kind := range []string{"python", "profile", "models"} {
		t.Run(kind, func(t *testing.T) {
			m, calls := cacheFixture(t)
			if err := os.WriteFile(filepath.Join(m.dir, "block"), []byte(kind), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				switch kind {
				case "python":
					done <- m.ensurePython(ctx, nil)
				case "profile":
					done <- m.ensureProfile(ctx, "test", nil)
				default:
					done <- m.prepareModels(ctx, "test", "test", nil)
				}
			}()
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, err := os.Stat(filepath.Join(m.dir, "entered")); err == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("cache check did not start")
				}
				time.Sleep(time.Millisecond)
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cache check did not stop")
			}
			if got := calls(); len(got) != 1 {
				t.Fatalf("cancel triggered repair: %+v", got)
			}
		})
	}
}
