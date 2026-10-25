package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/httpapi"
	"github.com/ozine/videohelper/internal/platform/authn"
	"github.com/ozine/videohelper/internal/workspace"
)

func runAPI(ctx context.Context, pool *pgxpool.Pool, app *runtime, cfg config.Config) error {
	verifier, err := authn.New(ctx, cfg.Auth)
	if err != nil {
		return err
	}
	ready := func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			return err
		}
		if err := app.redis.Ping(ctx).Err(); err != nil {
			return err
		}
		return app.channels.Validate(ctx, app.registry)
	}
	handler := httpapi.New(httpapi.Deps{Authn: verifier, Workspace: workspace.New(pool), Library: app.library,
		Pipeline: app.pipeline, Search: app.search, Blob: app.objects,
		Config: httpapi.Config{CORSOrigins: cfg.Server.CORSOrigins, PresignTTL: cfg.S3.PresignTTL}, Ready: ready})
	server := &http.Server{Addr: cfg.Server.Addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: max(30*time.Second, cfg.ML.QueryTimeout+15*time.Second),
		IdleTimeout: time.Minute, MaxHeaderBytes: 1 << 20, BaseContext: func(net.Listener) context.Context { return ctx }}
	defer server.Close()
	stopped := make(chan error, 1)
	go func() { stopped <- server.ListenAndServe() }()
	refresh := time.NewTicker(5 * time.Minute)
	defer refresh.Stop()
	for {
		select {
		case err := <-stopped:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			err := server.Shutdown(shutdown)
			cancel()
			if err != nil {
				err = errors.Join(err, server.Close())
			}
			serveErr := <-stopped
			if errors.Is(serveErr, http.ErrServerClosed) {
				serveErr = nil
			}
			return errors.Join(err, serveErr)
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
