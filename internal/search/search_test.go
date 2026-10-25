package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/platform/authn"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/internal/timeline"
	"github.com/ozine/videohelper/internal/workspace"
	"github.com/ozine/videohelper/migrations"
)

func testChannels(t *testing.T) *space.Channels {
	t.Helper()
	channels, err := space.NewChannels(config.ChannelsConfig{
		"visual": {Doc: "video", Seg: timeline.GridSeg, Weight: 1, Active: []string{"contract-test@r1"}},
		"speech": {Doc: "text", Seg: timeline.SentenceSeg, Weight: 1, Active: []string{"contract-test@r1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return channels
}

func TestQueryBoundaryAndEmptyScope(t *testing.T) {
	s := New(nil, library.New(nil, nil, library.Config{}, nil, nil), nil, testChannels(t), nil, config.SearchConfig{K: 10, RRFK: 60, MaxLimit: 50})
	p := workspace.Principal{UserID: 1, WorkspaceID: 1}
	for _, q := range []Query{{Text: " "}, {Text: "正常", Limit: 51}, {Text: "正常", TimeRange: &timeline.Span{StartMs: 1000, EndMs: 1000}}, {Text: "正常", OnlyChannels: []string{"audio"}}, {Text: "正常", ChannelWeights: map[string]float64{"visual": math.NaN()}}, {Text: "正常", ChannelQueries: map[string]string{"speech": " "}}} {
		if _, err := s.Search(context.Background(), p, q); apperr.KindOf(err) != apperr.Invalid {
			t.Fatalf("invalid query accepted: %+v %v", q, err)
		}
	}
	if _, err := s.Search(context.Background(), workspace.Principal{}, Query{Text: "正常"}); apperr.KindOf(err) != apperr.Unauthenticated {
		t.Fatalf("anonymous search: %v", err)
	}
	if result, err := s.Search(context.Background(), p, Query{Text: "正常", Scope: library.Scope{VideoIDs: []int64{}}}); err != nil || len(result.Moments) != 0 || result.Moments == nil || result.Skipped == nil {
		t.Fatalf("empty scope reached nil dependencies: %+v %v", result, err)
	}
}

func searchDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("VH_TEST_SEARCH_DB_URL")
	if dsn == "" {
		t.Skip("set VH_TEST_SEARCH_DB_URL to an empty videohelper_search_test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqldb.Close() })
	var name string
	var count int
	if err := sqldb.QueryRowContext(ctx, "SELECT current_database()").Scan(&name); err != nil || name != "videohelper_search_test" {
		t.Fatalf("refusing database %q: %v", name, err)
	}
	if err := sqldb.QueryRowContext(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename <> 'goose_db_version'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("refusing nonempty database: %d %v", count, err)
	}
	if err := migrations.Up(ctx, sqldb); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if err := migrations.DownTo(cleanup, sqldb, 0); err != nil {
			t.Error(err)
		}
	})
	pool, err := db.Open(ctx, dsn, 8)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return ctx, pool
}

func TestSearchPostgresIntegration(t *testing.T) {
	ctx, pool := searchDB(t)
	var metadata atomic.Value
	span := 30000
	fingerprint := "sha256:" + strings.Repeat("a", 64)
	metadata.Store(mlclient.SpacesResponse{Spaces: []mlclient.SpaceInfo{{ID: "contract-test@r1", Dim: 3, Metric: "cosine", QueryInputs: []string{"text"}, DocInputs: []string{"text", "video", "image"}, MaxSpanMs: &span, MaxBatch: 8, Fingerprint: fingerprint}}, Transcribers: []mlclient.TranscriberInfo{}})
	var embeds atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/spaces" {
			json.NewEncoder(w).Encode(metadata.Load())
			return
		}
		embeds.Add(1)
		var req mlclient.EmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Role != "query" || len(req.Inputs) != 1 || req.Inputs[0].Text == "" {
			t.Errorf("invalid text-only query encoding: %+v %v", req, err)
			w.WriteHeader(400)
			return
		}
		text := req.Inputs[0].Text
		if text == "故障查询" {
			w.WriteHeader(500)
			w.Write([]byte(`{"error":{"code":"internal","message":"explicit contract test failure"}}`))
			return
		}
		if text == "等待取消" {
			<-r.Context().Done()
			return
		}
		vector := []float32{1, 0, 0}
		if text == "另一查询" {
			vector = []float32{0, 1, 0}
		}
		info := metadata.Load().(mlclient.SpacesResponse).Spaces[0]
		json.NewEncoder(w).Encode(mlclient.EmbedResponse{Space: info.ID, Fingerprint: info.Fingerprint, Dim: 3, Vectors: [][]float32{vector}})
	}))
	defer server.Close()
	var registry *space.Registry
	ml := mlclient.New(config.MLConfig{Endpoints: []string{server.URL}, Token: "explicit-contract-test", QueryTimeout: time.Second}, func(id string) (string, bool) { return registry.Resolve(id) })
	registry = space.New(pool, ml)
	if err := registry.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	channels := testChannels(t)
	for _, channel := range channels.List() {
		if err := index.EnsureVectorIndex(ctx, pool, channel.Name, index.VectorSpace{ID: "contract-test@r1", Dim: 3, Metric: "cosine"}); err != nil {
			t.Fatal(err)
		}
	}
	workspaces := workspace.New(pool)
	p, err := workspaces.Resolve(ctx, authn.Claims{Sub: "search-owner"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := workspaces.Resolve(ctx, authn.Claims{Sub: "search-foreign"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	lb := library.New(pool, nil, library.Config{}, nil, nil)
	s := New(pool, lb, registry, channels, ml, config.SearchConfig{K: 10, RRFK: 60, MergeGapCells: 1, MaxLimit: 50})
	createVideo := func(workspaceID int64, status string, deleted bool) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, "INSERT INTO video(workspace_id,title,source_kind,status,duration_ms,deleted_at) VALUES($1,'test','upload',$2,5000,CASE WHEN $3 THEN now() END) RETURNING id", workspaceID, status, deleted).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	v1, v2 := createVideo(p.WorkspaceID, "ready", false), createVideo(p.WorkspaceID, "ready", false)
	hidden := []struct{ id, workspace int64 }{{createVideo(foreign.WorkspaceID, "ready", false), foreign.WorkspaceID}, {createVideo(p.WorkspaceID, "indexing", false), p.WorkspaceID}, {createVideo(p.WorkspaceID, "ready", true), p.WorkspaceID}}
	rows := []index.VectorRow{
		{VideoID: v1, WorkspaceID: p.WorkspaceID, Channel: "visual", SpaceID: "contract-test@r1", Seg: timeline.GridSeg, Span: timeline.Span{StartMs: 0, EndMs: 1000}, Vec: []float32{1, 0, 0}},
		{VideoID: v1, WorkspaceID: p.WorkspaceID, Channel: "visual", SpaceID: "contract-test@r1", Seg: timeline.GridSeg, Span: timeline.Span{StartMs: 3000, EndMs: 4000}, Vec: []float32{0.8, 0.6, 0}},
		{VideoID: v1, WorkspaceID: p.WorkspaceID, Channel: "speech", SpaceID: "contract-test@r1", Seg: timeline.SentenceSeg, Span: timeline.Span{StartMs: 500, EndMs: 1500}, Vec: []float32{1, 0, 0}},
		{VideoID: v2, WorkspaceID: p.WorkspaceID, Channel: "visual", SpaceID: "contract-test@r1", Seg: timeline.GridSeg, Span: timeline.Span{StartMs: 4000, EndMs: 5000}, Vec: []float32{0, 1, 0}},
	}
	for _, video := range hidden {
		rows = append(rows, index.VectorRow{VideoID: video.id, WorkspaceID: video.workspace, Channel: "visual", SpaceID: "contract-test@r1", Seg: timeline.GridSeg, Span: timeline.Span{StartMs: 0, EndMs: 1000}, Vec: []float32{1, 0, 0}})
	}
	if err := index.UpsertVectors(ctx, pool, rows); err != nil {
		t.Fatal(err)
	}
	original := "你好，这是保留原文的中文证据。"
	if err := db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		return index.ReplaceSpeech(ctx, tx, p.WorkspaceID, v1, "test-asr", []index.TextSpan{{Span: timeline.Span{StartMs: 500, EndMs: 1500}, Text: original}})
	}); err != nil {
		t.Fatal(err)
	}
	// ReplaceSpeech invalidates old speech vectors; the fixture explicitly rebuilds its one vector.
	if err := index.UpsertVectors(ctx, pool, rows[2:3]); err != nil {
		t.Fatal(err)
	}
	q := Query{Text: "主查询"}
	before := embeds.Load()
	result, err := s.Search(ctx, p, q)
	if err != nil || len(result.Moments) != 2 || len(result.Skipped) != 0 || embeds.Load()-before != 1 {
		t.Fatalf("same-space dedup/search: %+v %v embeds=%d", result, err, embeds.Load()-before)
	}
	best := result.Moments[0]
	if best.VideoID != v1 || best.JumpMs != 500 || best.Snippet != original || best.ThumbCell != 0 || best.Span != (timeline.Span{StartMs: 0, EndMs: 4000}) {
		t.Fatalf("ranked Chinese evidence: %+v", best)
	}
	for _, moment := range result.Moments {
		if moment.VideoID != v1 && moment.VideoID != v2 {
			t.Fatalf("hidden video leaked: %+v", moment)
		}
	}

	t.Run("scope and empty project", func(t *testing.T) {
		project, err := lb.CreateProject(ctx, p, "空项目")
		if err != nil {
			t.Fatal(err)
		}
		before := embeds.Load()
		result, err := s.Search(ctx, p, Query{Text: "主查询", Scope: library.Scope{ProjectID: &project.ID}})
		if err != nil || len(result.Moments) != 0 || embeds.Load() != before {
			t.Fatalf("empty scope encoded query: %+v %v", result, err)
		}
		if err := lb.AddVideo(ctx, p, project.ID, v2); err != nil {
			t.Fatal(err)
		}
		result, err = s.Search(ctx, p, Query{Text: "主查询", Scope: library.Scope{ProjectID: &project.ID}})
		if err != nil || len(result.Moments) != 1 || result.Moments[0].VideoID != v2 {
			t.Fatalf("project scope: %+v %v", result, err)
		}
		if _, err := s.Search(ctx, p, Query{Text: "主查询", Scope: library.Scope{VideoIDs: []int64{hidden[0].id}}}); apperr.KindOf(err) != apperr.NotFound {
			t.Fatalf("foreign explicit scope: %v", err)
		}
	})
	t.Run("half-open time clip", func(t *testing.T) {
		window := timeline.Span{StartMs: 1000, EndMs: 2000}
		result, err := s.Search(ctx, p, Query{Text: "主查询", TimeRange: &window})
		if err != nil || len(result.Moments) != 1 || result.Moments[0].Span != window || result.Moments[0].JumpMs != 1000 || result.Moments[0].Snippet != original {
			t.Fatalf("clipped speech evidence: %+v %v", result, err)
		}
		result, err = s.Search(ctx, p, Query{Text: "主查询", TimeRange: &window, OnlyChannels: []string{"visual"}})
		if err != nil || len(result.Moments) != 0 {
			t.Fatalf("touching visual range contributed: %+v %v", result, err)
		}
	})
	t.Run("channel queries weights and partial failure", func(t *testing.T) {
		before := embeds.Load()
		result, err := s.Search(ctx, p, Query{Text: "主查询", ChannelQueries: map[string]string{"speech": "另一查询"}})
		if err != nil || embeds.Load()-before != 2 || len(result.Moments) == 0 {
			t.Fatalf("channel-specific query dedup: %+v %v", result, err)
		}
		result, err = s.Search(ctx, p, Query{Text: "主查询", ChannelWeights: map[string]float64{"visual": 0}})
		if err != nil || len(result.Moments) != 1 || len(result.Moments[0].Channels) != 1 || result.Moments[0].Channels[0].Channel != "speech" || len(result.Skipped) != 0 {
			t.Fatalf("zero weight: %+v %v", result, err)
		}
		result, err = s.Search(ctx, p, Query{Text: "主查询", ChannelQueries: map[string]string{"visual": "故障查询"}})
		if err != nil || len(result.Moments) != 1 || !slices.Equal(result.Skipped, []string{"visual"}) {
			t.Fatalf("partial encoding failure: %+v %v", result, err)
		}
		result, err = s.Search(ctx, p, Query{Text: "故障查询"})
		if apperr.KindOf(err) != apperr.Unavailable || !slices.Equal(result.Skipped, []string{"speech", "visual"}) {
			t.Fatalf("all encoding failed: %+v %v", result, err)
		}
		result, err = s.Search(ctx, p, Query{Text: "主查询", SpaceOverride: map[string]string{"visual": "missing@r1"}})
		if err != nil || len(result.Moments) != 1 || !slices.Equal(result.Skipped, []string{"visual"}) {
			t.Fatalf("unavailable route did not preserve healthy channel: %+v %v", result, err)
		}
	})
	t.Run("caller cancellation", func(t *testing.T) {
		short, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
		defer cancel()
		if _, err := s.Search(short, p, Query{Text: "等待取消"}); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("search cancellation lost: %v", err)
		}
	})
	t.Run("ML metadata refresh cannot bypass registered fingerprint", func(t *testing.T) {
		changed := metadata.Load().(mlclient.SpacesResponse)
		changed.Spaces = slices.Clone(changed.Spaces)
		changed.Spaces[0].Fingerprint = "sha256:" + strings.Repeat("b", 64)
		metadata.Store(changed)
		if _, err := ml.Spaces(ctx, server.URL); err != nil {
			t.Fatal(err)
		}
		result, err := s.Search(ctx, p, Query{Text: "主查询"})
		if apperr.KindOf(err) != apperr.Unavailable || !errors.Is(err, mlclient.ErrContract) || len(result.Moments) != 0 {
			t.Fatalf("registered fingerprint bypassed: %+v %v", result, err)
		}
	})
}
