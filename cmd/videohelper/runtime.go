package main

import (
	"context"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/pipeline"
	"github.com/ozine/videohelper/internal/pipeline/stages"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/search"
	"github.com/ozine/videohelper/internal/space"
	"github.com/redis/go-redis/v9"
)

type runtime struct {
	objects   *blob.Store
	library   *library.Service
	search    *search.Service
	pipeline  *pipeline.Service
	registry  *space.Registry
	channels  *space.Channels
	queue     *asynq.Client
	inspector *asynq.Inspector
	redis     *redis.Client
	http      *http.Client
}

func commandHTTPClient() *http.Client {
	return &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone(),
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func (r *runtime) close() {
	if r.queue != nil {
		r.queue.Close()
	}
	if r.inspector != nil {
		r.inspector.Close()
	}
	if r.redis != nil {
		r.redis.Close()
	}
	if r.http != nil {
		r.http.CloseIdleConnections()
	}
}

func modelRegistry(ctx context.Context, pool *pgxpool.Pool, cfg config.MLConfig) (*mlclient.Client, *space.Registry, error) {
	var registry *space.Registry
	client := mlclient.New(cfg, func(id string) (string, bool) { return registry.Resolve(id) })
	registry = space.New(pool, client)
	return client, registry, registry.Sync(ctx)
}

func newRuntime(ctx context.Context, pool *pgxpool.Pool, cfg config.Config, role config.Role) (*runtime, error) {
	app := &runtime{http: commandHTTPClient()}
	ready := false
	defer func() {
		if !ready {
			app.close()
		}
	}()
	ml, registry, err := modelRegistry(ctx, pool, cfg.ML)
	if err != nil {
		return nil, err
	}
	app.registry = registry
	app.channels, err = space.NewChannels(cfg.Channels)
	if err != nil {
		return nil, err
	}
	if err := app.channels.Validate(ctx, registry); err != nil {
		return nil, err
	}
	objects, err := objectStore(ctx, cfg.S3, app.http)
	if err != nil {
		return nil, err
	}
	app.objects = objects
	if role != config.RoleEval {
		app.redis = redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr})
		if err := app.redis.Ping(ctx).Err(); err != nil {
			return nil, err
		}
		option := asynq.RedisClientOpt{Addr: cfg.Redis.Addr}
		app.queue, app.inspector = asynq.NewClient(option), asynq.NewInspector(option)
		runs := stages.New(pool, objects, ml, registry, stages.Config{Media: cfg.Media, ML: cfg.ML, PresignTTL: cfg.S3.PresignTTL}).Runs()
		app.pipeline = pipeline.New(pipeline.Deps{Pool: pool, Client: app.queue, Inspector: app.inspector,
			Redis: app.redis, Channels: app.channels, Registry: registry, Worker: cfg.Worker, Runs: runs})
	}
	app.library = library.New(pool, objects, library.Config{UploadPartBytes: cfg.Limits.UploadPartBytes, PresignTTL: cfg.S3.PresignTTL}, app.pipeline, app.pipeline)
	app.search = search.New(pool, app.library, registry, app.channels, ml, cfg.Search)
	ready = true
	return app, nil
}

func objectStore(ctx context.Context, cfg config.S3Config, client *http.Client) (*blob.Store, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region), awsconfig.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}
	sdk := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if cfg.Endpoint != "" {
			options.BaseEndpoint = aws.String(cfg.Endpoint)
			options.UsePathStyle = true
		}
	})
	return blob.New(sdk, cfg.Bucket), nil
}
