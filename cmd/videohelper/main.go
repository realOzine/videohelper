package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/pipeline"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/search"
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
		fmt.Fprintln(flags.Output(), "Usage: videohelper [-config path] migrate | local-init | worker | api | local-upload <file> | local-search <text> | local-video <id> | space add <channel> <id>")
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
	role, err := commandRole(args)
	if err != nil {
		return err
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
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
	defer http.DefaultClient.CloseIdleConnections()
	if args[0] == "local-init" {
		if cfg.S3.Endpoint == "" {
			return errors.New("local-init requires an explicit local S3 endpoint")
		}
		client := commandHTTPClient()
		defer client.CloseIdleConnections()
		objects, err := objectStore(ctx, cfg.S3, client)
		if err != nil {
			return err
		}
		if err := objects.EnsureBucket(ctx); err != nil {
			return err
		}
		principal, err := localPrincipal(ctx, pool)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"workspace_id": fmt.Sprint(principal.WorkspaceID), "user_id": fmt.Sprint(principal.UserID)})
	}
	if args[0] == "local-video" {
		principal, err := localPrincipal(ctx, pool)
		if err != nil {
			return err
		}
		id, _ := strconv.ParseInt(args[1], 10, 64)
		video, err := library.New(pool, nil, library.Config{}, nil, nil).Get(ctx, principal, id)
		if err != nil {
			return err
		}
		stages, err := pipeline.New(pipeline.Deps{Pool: pool}).ListStages(ctx, principal.WorkspaceID, id)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(struct {
			Video  library.Video          `json:"video"`
			Stages []pipeline.StageStatus `json:"stages"`
		}{video, stages})
	}
	if args[0] != "space" {
		app, err := newRuntime(ctx, pool, cfg, role)
		if err != nil {
			return err
		}
		defer app.close()
		if args[0] == "api" {
			return runAPI(ctx, pool, app, cfg)
		}
		if args[0] == "worker" {
			return runWorker(ctx, app, cfg)
		}
		principal, err := localPrincipal(ctx, pool)
		if err != nil {
			return err
		}
		var result any
		if args[0] == "local-upload" {
			result, err = localUpload(ctx, app, principal, args[1])
		} else {
			result, err = app.search.Search(ctx, principal, search.Query{Text: args[1]})
		}
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	_, registry, err := modelRegistry(ctx, pool, cfg.ML)
	if err != nil {
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

func commandRole(args []string) (config.Role, error) {
	if len(args) == 1 {
		switch args[0] {
		case "migrate", "local-init":
			return config.RoleMigrate, nil
		case "worker":
			return config.RoleWorker, nil
		case "api":
			return config.RoleAPI, nil
		}
	}
	if len(args) == 2 && strings.TrimSpace(args[1]) != "" {
		switch args[0] {
		case "local-upload":
			return config.RoleWorker, nil
		case "local-search":
			return config.RoleEval, nil
		case "local-video":
			id, err := strconv.ParseInt(args[1], 10, 64)
			if err == nil && id > 0 {
				return config.RoleMigrate, nil
			}
		}
	}
	if len(args) == 4 && args[0] == "space" && args[1] == "add" {
		return config.RoleSpace, nil
	}
	return "", errors.New("unknown command or invalid arguments; run videohelper -help")
}
