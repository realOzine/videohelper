package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/platform/queue"
)

func runWorker(ctx context.Context, app *runtime, cfg config.Config) error {
	mux := asynq.NewServeMux()
	app.pipeline.Register(mux)
	handler := asynq.HandlerFunc(func(taskCtx context.Context, task *asynq.Task) error {
		linked, cancel := context.WithCancel(taskCtx)
		stop := context.AfterFunc(ctx, cancel)
		defer func() { stop(); cancel() }()
		return mux.ProcessTask(linked, task)
	})
	cpu, gpu := queue.NewServers(cfg.Worker, asynq.RedisClientOpt{Addr: cfg.Redis.Addr})
	if err := cpu.Start(handler); err != nil {
		return err
	}
	defer cpu.Shutdown()
	if err := gpu.Start(handler); err != nil {
		return err
	}
	defer gpu.Shutdown()
	if err := app.pipeline.Reconcile(ctx); err != nil {
		slog.ErrorContext(ctx, "startup reconciliation failed", "error", err)
	}
	reconcile := time.NewTicker(cfg.Worker.ReconcileEvery)
	defer reconcile.Stop()
	refresh := time.NewTicker(5 * time.Minute)
	defer refresh.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-reconcile.C:
			if err := queue.EnqueueReconcile(ctx, app.queue, now, cfg.Worker.ReconcileEvery); err != nil {
				slog.ErrorContext(ctx, "enqueue reconciliation failed", "error", err)
			}
		case <-refresh.C:
			err := app.registry.Sync(ctx)
			if err == nil {
				err = app.channels.Validate(ctx, app.registry)
			}
			if err != nil {
				slog.ErrorContext(ctx, "ML registry refresh failed", "error", err)
			}
		}
	}
}
