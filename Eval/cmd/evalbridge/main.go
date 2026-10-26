package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/search"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/internal/workspace"
	pgxvec "github.com/pgvector/pgvector-go/pgx"
)

type request struct {
	WorkspaceID    int64              `json:"workspace_id"`
	UserID         int64              `json:"user_id"`
	VideoIDs       []int64            `json:"video_ids"`
	QueryID        string             `json:"query_id,omitempty"`
	Mode           string             `json:"mode,omitempty"`
	Text           string             `json:"text,omitempty"`
	Limit          int                `json:"limit,omitempty"`
	ChannelWeights map[string]float64 `json:"channel_weights,omitempty"`
}

func (r *request) validate(command string) error {
	if r.WorkspaceID <= 0 || r.UserID <= 0 || len(r.VideoIDs) == 0 {
		return fmt.Errorf("positive workspace_id, user_id and nonempty video_ids are required")
	}
	slices.Sort(r.VideoIDs)
	for i, id := range r.VideoIDs {
		if id <= 0 || i > 0 && id == r.VideoIDs[i-1] {
			return fmt.Errorf("video_ids must be positive and unique")
		}
	}
	if command == "search" {
		if r.QueryID == "" || strings.TrimSpace(r.Text) == "" || r.Limit <= 0 {
			return fmt.Errorf("query_id, nonempty text and positive limit are required")
		}
		if r.Mode != "visual" && r.Mode != "speech" && r.Mode != "fused" {
			return fmt.Errorf("unknown search mode %q", r.Mode)
		}
		if r.Mode != "fused" && r.ChannelWeights != nil {
			return fmt.Errorf("channel_weights requires fused search")
		}
	} else if r.QueryID != "" || r.Mode != "" || r.Text != "" || r.Limit != 0 || r.ChannelWeights != nil {
		return fmt.Errorf("inspect accepts only scope fields")
	}
	return nil
}

func (r request) validateWeights(channels *space.Channels) error {
	for name, weight := range r.ChannelWeights {
		channel, ok := channels.Get(name)
		if !ok || weight < 0 || math.IsNaN(weight) || math.IsInf(weight, 0) || math.IsInf(weight*channel.Weight, 0) {
			return fmt.Errorf("invalid channel weight %q", name)
		}
	}
	for _, channel := range channels.List() {
		weight := channel.Weight
		if multiplier, ok := r.ChannelWeights[channel.Name]; ok {
			weight *= multiplier
		}
		if weight > 0 {
			return nil
		}
	}
	return fmt.Errorf("at least one effective channel weight must be positive")
}

type channelHit struct {
	Channel string `json:"channel"`
	Rank    int    `json:"rank"`
}
type moment struct {
	VideoID  int64        `json:"video_id"`
	StartMs  int          `json:"start_ms"`
	EndMs    int          `json:"end_ms"`
	JumpMs   int          `json:"jump_ms"`
	Score    float64      `json:"score"`
	Channels []channelHit `json:"channels"`
	Snippet  string       `json:"snippet"`
}
type queryError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type searchResponse struct {
	QueryID   string      `json:"query_id"`
	Mode      string      `json:"mode"`
	LatencyMs float64     `json:"latency_ms"`
	Moments   []moment    `json:"moments"`
	Skipped   []string    `json:"skipped"`
	Error     *queryError `json:"error"`
}
type searcher interface {
	Search(context.Context, workspace.Principal, search.Query) (search.Result, error)
}

func searchOne(ctx context.Context, service searcher, r request) searchResponse {
	start := time.Now()
	q := search.Query{Text: r.Text, Scope: library.Scope{VideoIDs: r.VideoIDs}, Limit: r.Limit}
	if r.Mode == "fused" {
		q.ChannelWeights = r.ChannelWeights
	} else {
		q.OnlyChannels = []string{r.Mode}
		q.ChannelWeights = map[string]float64{r.Mode: 1}
	}
	result, err := service.Search(ctx, workspace.Principal{UserID: r.UserID, WorkspaceID: r.WorkspaceID}, q)
	out := searchResponse{QueryID: r.QueryID, Mode: r.Mode, LatencyMs: float64(time.Since(start).Microseconds()) / 1000, Moments: []moment{}, Skipped: result.Skipped}
	if out.Skipped == nil {
		out.Skipped = []string{}
	}
	if err != nil {
		out.Error = &queryError{Code: "search_failed", Message: err.Error()}
		var app *apperr.Error
		if errors.As(err, &app) {
			out.Error.Code = app.Code
		}
		return out
	}
	for _, m := range result.Moments {
		channels := make([]channelHit, 0, len(m.Channels))
		for _, c := range m.Channels {
			channels = append(channels, channelHit{c.Channel, c.Rank})
		}
		out.Moments = append(out.Moments, moment{m.VideoID, m.Span.StartMs, m.Span.EndMs, m.JumpMs, m.Score, channels, m.Snippet})
	}
	return out
}

func readOnlyPool(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	c, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, err
	}
	c.MaxConns = int32(cfg.MaxConns)
	c.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	c.AfterConnect = pgxvec.RegisterTypes
	p, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return nil, err
	}
	if err := p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return p, nil
}

func run(ctx context.Context, args []string, input io.Reader, output io.Writer) error {
	flags := flag.NewFlagSet("evalbridge", flag.ContinueOnError)
	path := flags.String("config", "", "application config path")
	k := flags.Int("k", 0, "override search k")
	rrf := flags.Float64("rrf-k", 0, "override search rrf_k")
	gap := flags.Int("merge-gap", -1, "override search merge_gap_cells")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" || flags.NArg() != 1 || flags.Arg(0) != "inspect" && flags.Arg(0) != "search" {
		return fmt.Errorf("usage: evalbridge -config <path> [-k N -rrf-k N -merge-gap N] inspect|search")
	}
	if *k < 0 || *rrf < 0 || math.IsNaN(*rrf) || math.IsInf(*rrf, 0) || *gap < -1 {
		return fmt.Errorf("invalid search override")
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	if *k > 0 {
		cfg.Search.K = *k
	}
	if *rrf > 0 {
		cfg.Search.RRFK = *rrf
	}
	if *gap >= 0 {
		cfg.Search.MergeGapCells = *gap
	}
	if err := cfg.Validate(config.RoleEval); err != nil {
		return err
	}
	channels, err := space.NewChannels(cfg.Channels)
	if err != nil {
		return err
	}
	command := flags.Arg(0)
	var registry *space.Registry
	client := mlclient.New(cfg.ML, func(id string) (string, bool) { return registry.Resolve(id) })
	var pool *pgxpool.Pool
	var service *search.Service
	defer func() {
		if pool != nil {
			pool.Close()
		}
	}()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	encoder := json.NewEncoder(output)
	count := 0
	for scanner.Scan() {
		count++
		var r request
		d := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		d.DisallowUnknownFields()
		if err := d.Decode(&r); err != nil {
			return fmt.Errorf("line %d: %w", count, err)
		}
		if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
			return fmt.Errorf("line %d: expected one JSON object", count)
		}
		if err := r.validate(command); err != nil {
			return fmt.Errorf("line %d: %w", count, err)
		}
		if err := r.validateWeights(channels); err != nil {
			return fmt.Errorf("line %d: %w", count, err)
		}
		if pool == nil {
			if command == "search" {
				// Registry initialization uses the application's existing metadata registration path.
				p, err := db.Open(ctx, cfg.Database.URL, cfg.Database.MaxConns)
				if err != nil {
					return err
				}
				registry = space.New(p, client, cfg.ML)
				err = registry.Sync(ctx)
				if err == nil {
					err = channels.Validate(ctx, registry)
				}
				p.Close()
				if err != nil {
					return err
				}
			}
			pool, err = readOnlyPool(ctx, cfg.Database)
			if err != nil {
				return err
			}
			service = search.New(pool, library.New(pool, nil, library.Config{}, nil, nil), registry, channels, client, cfg.Search)
		}
		var response any
		if command == "search" {
			response = searchOne(ctx, service, r)
		} else {
			response, err = inspect(ctx, pool, client, channels, r)
			if err != nil {
				return fmt.Errorf("line %d: %w", count, err)
			}
		}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("at least one JSON request is required")
	}
	return nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
