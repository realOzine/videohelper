package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/platform/authn"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/internal/workspace"
	"github.com/ozine/videohelper/migrations"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil && !errors.Is(err, flag.ErrHelp) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("videohelper", flag.ContinueOnError)
	path := flags.String("config", "config/local.yaml", "configuration file (run from repository root)")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: videohelper [-config path] migrate | local-init | space add <channel> <id>")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return err
	}
	args = flags.Args()
	if len(args) == 0 {
		flags.Usage()
		return flag.ErrHelp
	}
	if (len(args) != 1 || (args[0] != "migrate" && args[0] != "local-init")) && (len(args) != 4 || args[0] != "space" || args[1] != "add") {
		return errors.New("unknown command; run videohelper -help")
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	role := config.RoleMigrate
	if args[0] == "space" {
		role = config.RoleSpace
	}
	if err := cfg.Validate(role); err != nil {
		return err
	}
	if args[0] == "migrate" {
		database, err := sql.Open("pgx", cfg.Database.URL)
		if err != nil {
			return err
		}
		defer database.Close()
		return migrations.Up(ctx, database)
	}
	pool, err := db.Open(ctx, cfg.Database.URL, cfg.Database.MaxConns)
	if err != nil {
		return err
	}
	defer pool.Close()
	if args[0] == "local-init" {
		if cfg.S3.Endpoint == "" {
			return errors.New("local-init requires an explicit local S3 endpoint")
		}
		objects, err := objectStore(ctx, cfg.S3)
		if err != nil {
			return err
		}
		if err := objects.EnsureBucket(ctx); err != nil {
			return err
		}
		principal, err := workspace.New(pool).Resolve(ctx, authn.Claims{Sub: "videohelper-local", Name: "Local", Email: "local@videohelper"}, 0)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"workspace_id": fmt.Sprint(principal.WorkspaceID), "user_id": fmt.Sprint(principal.UserID)})
	}
	var registry *space.Registry
	client := mlclient.New(cfg.ML, func(id string) (string, bool) { return registry.Resolve(id) })
	registry = space.New(pool, client)
	if err := registry.Sync(ctx); err != nil {
		return err
	}
	channel, id := args[2], args[3]
	doc := map[string]string{"visual": "video", "speech": "text"}[channel]
	if doc == "" {
		return fmt.Errorf("unsupported channel %q", channel)
	}
	info, ok := registry.Get(id)
	if !ok || info.Disabled || !slices.Contains(info.DocInputs, doc) || !slices.Contains(info.QueryInputs, "text") {
		return fmt.Errorf("space %s is unavailable or incompatible with channel %s", id, channel)
	}
	if err := index.EnsureVectorIndex(ctx, pool, channel, index.VectorSpace{ID: info.ID, Dim: info.Dim, Metric: info.Metric}); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]string{"channel": channel, "space": id, "status": "indexed"})
}

func objectStore(ctx context.Context, cfg config.S3Config) (*blob.Store, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.Region))
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if cfg.Endpoint != "" {
			options.BaseEndpoint = aws.String(cfg.Endpoint)
			options.UsePathStyle = true
		}
	})
	return blob.New(client, cfg.Bucket), nil
}
