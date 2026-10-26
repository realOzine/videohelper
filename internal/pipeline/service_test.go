package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/pipeline/pipelinedb"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/platform/queue"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/migrations"
	"github.com/redis/go-redis/v9"
)

func pipelineFixture(t *testing.T) (context.Context, *Service) {
	t.Helper()
	dsn, redisAddr := os.Getenv("VH_TEST_PIPELINE_DB_URL"), os.Getenv("VH_TEST_REDIS_ADDR")
	if dsn == "" || redisAddr == "" {
		t.Skip("set VH_TEST_PIPELINE_DB_URL and VH_TEST_REDIS_ADDR for dedicated PostgreSQL/Redis integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqldb.Close() })
	var name string
	if err := sqldb.QueryRowContext(ctx, "SELECT current_database()").Scan(&name); err != nil || name != "videohelper_pipeline_test" {
		t.Fatalf("refusing database %q: %v", name, err)
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
	r := redis.NewClient(&redis.Options{Addr: redisAddr, DB: 14})
	if err := r.FlushDB(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := r.FlushDB(cleanup).Err(); err != nil {
			t.Error(err)
		}
		r.Close()
	})
	opt := asynq.RedisClientOpt{Addr: redisAddr, DB: 14}
	client, inspector := asynq.NewClient(opt), asynq.NewInspector(opt)
	t.Cleanup(func() { client.Close(); inspector.Close() })
	metadata := mlclient.SpacesResponse{Spaces: []mlclient.SpaceInfo{}, Transcribers: []mlclient.TranscriberInfo{{ID: "asr@r1", Languages: []string{"zh"}, MaxWindowMs: 900000, Fingerprint: "sha256:" + strings.Repeat("b", 64)}}}
	maxSpan := 30000
	for _, id := range []string{"active@r1", "shadow@r1"} {
		metadata.Spaces = append(metadata.Spaces, mlclient.SpaceInfo{ID: id, Dim: 3, Metric: "cosine", QueryInputs: []string{"text"}, DocInputs: []string{"video", "text", "image"}, MaxSpanMs: &maxSpan, MaxBatch: 32, Fingerprint: "sha256:" + strings.Repeat("a", 64)})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(metadata) }))
	t.Cleanup(server.Close)
	registry := space.New(pool, mlclient.New(config.MLConfig{Endpoints: []string{server.URL}, Token: "test"}, nil), config.MLConfig{Transcriber: "asr@r1"})
	if err := registry.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	channels, err := space.NewChannels(config.ChannelsConfig{
		"visual": {Doc: "video", Seg: "grid@v1", Weight: 1, Active: []string{"active@r1"}, Shadow: []string{"shadow@r1"}},
		"speech": {Doc: "text", Seg: "sentence@v1", Weight: 1, Active: []string{"active@r1"}, Shadow: []string{"shadow@r1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, New(Deps{Pool: pool, Client: client, Inspector: inspector, Redis: r, Channels: channels, Registry: registry,
		Worker: config.WorkerConfig{MaxRetry: 2, WorkspaceGPULimit: 1}, Runs: map[string]RunFunc{}})
}

func uploadFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (workspaceID, videoID int64) {
	t.Helper()
	if err := pool.QueryRow(ctx, "INSERT INTO workspace(name) VALUES('pipeline') RETURNING id").Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO video(workspace_id,title,source_kind,status,source_key) VALUES($1,'upload','upload','uploading','source.mp4') RETURNING id", workspaceID).Scan(&videoID); err != nil {
		t.Fatal(err)
	}
	return
}

func onlyStage(t *testing.T, ctx context.Context, s *Service, videoID int64, name string) pipelinedb.StageRun {
	t.Helper()
	rows, err := pipelinedb.New(s.deps.Pool).Stages(ctx, videoID)
	if err != nil {
		t.Fatal(err)
	}
	var found []pipelinedb.StageRun
	for _, row := range rows {
		if row.Stage == name {
			found = append(found, row)
		}
	}
	if len(found) != 1 {
		t.Fatalf("stage %s count=%d", name, len(found))
	}
	return found[0]
}

func TestPlanAndStartIntegration(t *testing.T) {
	ctx, service := pipelineFixture(t)
	workspaceID, videoID := uploadFixture(t, ctx, service.deps.Pool)
	for range 2 {
		if err := service.Start(ctx, videoID); err != nil {
			t.Fatal(err)
		}
	}
	probe := onlyStage(t, ctx, service, videoID, "probe")
	if probe.Status != "queued" || !strings.HasPrefix(probe.Version, "v1.") || probe.Attempt != 0 {
		t.Fatalf("initial stage=%+v", probe)
	}
	payload := queue.StagePayload{WorkspaceID: workspaceID, VideoID: videoID, Stage: probe.Stage, Version: probe.Version}
	if _, err := service.deps.Inspector.GetTaskInfo(queue.QueueCPU, queue.StageTaskID(payload)); err != nil {
		t.Fatal(err)
	}
	err := db.WithTx(ctx, service.deps.Pool, func(tx pgx.Tx) error {
		video, err := library.GetForWorker(ctx, tx, videoID)
		if err != nil {
			return err
		}
		yes := true
		video.HasAudio, video.HasVideo = &yes, &yes
		plan, err := ExpandPlan(video, service.deps.Channels.List(), service.deps.Registry)
		if err != nil {
			return err
		}
		var names []string
		for _, def := range plan {
			names = append(names, def.Name)
			if strings.Contains(def.Name, "shadow") && (def.Required || def.Queue != queue.QueueGPULow) {
				t.Error("shadow stage blocks readiness or uses interactive queue")
			}
			if def.Name == "finalize" && (slices.Contains(def.Deps, "embed:visual:shadow@r1") || !slices.Contains(def.Deps, "embed:speech:active@r1") || !slices.Contains(def.Deps, "asr")) {
				t.Fatalf("finalize dependencies=%v", def.Deps)
			}
		}
		if len(names) != 7 || slices.Contains(names, "embed:audio:active@r1") {
			t.Fatalf("expanded plan=%v", names)
		}
		return addPlan(ctx, tx, video, plan)
	})
	if err != nil {
		t.Fatal(err)
	}
	visual := onlyStage(t, ctx, service, videoID, "embed:visual:active@r1")
	output, err := decodeOutput(visual.Output)
	if err != nil || output.Plan.Options.Space.Fingerprint != "sha256:"+strings.Repeat("a", 64) || output.Plan.Options.Seg != "grid@v1" {
		t.Fatalf("frozen plan=%+v error=%v", output, err)
	}
}

func TestRetryAndDropGenerationIntegration(t *testing.T) {
	ctx, service := pipelineFixture(t)
	workspaceID, videoID := uploadFixture(t, ctx, service.deps.Pool)
	if err := service.Start(ctx, videoID); err != nil {
		t.Fatal(err)
	}
	before := onlyStage(t, ctx, service, videoID, "probe")
	oldID := queue.StageTaskID(queue.StagePayload{WorkspaceID: workspaceID, VideoID: videoID, Stage: before.Stage, Version: before.Version})
	if err := service.deps.Inspector.ArchiveTask(queue.QueueCPU, oldID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.deps.Pool.Exec(ctx, "UPDATE video SET status='failed' WHERE id=$1;", videoID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.deps.Pool.Exec(ctx, "UPDATE stage_run SET status='failed' WHERE video_id=$1", videoID); err != nil {
		t.Fatal(err)
	}
	if err := service.Retry(ctx, workspaceID+1, videoID); apperr.KindOf(err) != apperr.NotFound {
		t.Fatalf("foreign retry=%v", err)
	}
	if err := service.Retry(ctx, workspaceID, videoID); err != nil {
		t.Fatal(err)
	}
	after := onlyStage(t, ctx, service, videoID, "probe")
	if before.Version == after.Version || after.Status != "queued" || after.Attempt != 0 {
		t.Fatalf("retry did not retire generation: before=%+v after=%+v", before, after)
	}
	if _, err := service.deps.Inspector.GetTaskInfo(queue.QueueCPU, oldID); !errors.Is(err, asynq.ErrTaskNotFound) {
		t.Fatalf("archived old task remains: %v", err)
	}
	if err := service.Retry(ctx, workspaceID, videoID); apperr.KindOf(err) != apperr.Conflict {
		t.Fatalf("nonfailed retry=%v", err)
	}
	if err := db.WithTx(ctx, service.deps.Pool, func(tx pgx.Tx) error {
		if _, err := library.GetForWorker(ctx, tx, videoID); err != nil {
			return err
		}
		return service.DropDerived(ctx, tx, videoID)
	}); err != nil {
		t.Fatal(err)
	}
	if err := service.Start(ctx, videoID); err != nil {
		t.Fatal(err)
	}
	restored := onlyStage(t, ctx, service, videoID, "probe")
	if restored.Version == after.Version || restored.Version == before.Version {
		t.Fatal("restored video reused a deleted generation")
	}
}
