package main

import (
	"context"
	"errors"
	"time"

	desktopruntime "github.com/ozine/videohelper/internal/desktop/runtime"
)

// A model process can die while the processor is idle between durable jobs.
func superviseProcessor(ctx context.Context, run func(context.Context) error, state func() desktopruntime.State) error {
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- run(workerCtx) }()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			cancel()
			<-done
			return ctx.Err()
		case <-tick.C:
			s := state()
			if s.Ready {
				continue
			}
			cancel()
			<-done
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if s.Error == "" {
				s.Error = "Local model service stopped"
			}
			return errors.New(s.Error)
		}
	}
}
