package main

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	desktopruntime "github.com/ozine/videohelper/internal/desktop/runtime"
)

func TestModelExitCancelsAndWaitsForProcessor(t *testing.T) {
	var ready, stopped atomic.Bool
	ready.Store(true)
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- superviseProcessor(t.Context(), func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			stopped.Store(true)
			return nil
		}, func() desktopruntime.State {
			return desktopruntime.State{Ready: ready.Load(), Error: "ML process exited"}
		})
	}()
	<-started
	ready.Store(false)
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "ML process exited") {
			t.Fatalf("model exit lost: %v", err)
		}
		if !stopped.Load() {
			t.Fatal("processor was not stopped before allowing runtime replacement")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("model exit did not release the processor")
	}
}

func TestClosingAppWaitsForProcessor(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var stopped atomic.Bool
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- superviseProcessor(ctx, func(ctx context.Context) error { close(started); <-ctx.Done(); stopped.Store(true); return nil }, func() desktopruntime.State { return desktopruntime.State{Ready: true} })
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) || !stopped.Load() {
		t.Fatalf("shutdown did not join processor: %v", err)
	}
}
