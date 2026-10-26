package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/pipeline/pipelinedb"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/platform/queue"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/internal/timeline"
	"github.com/ozine/videohelper/internal/workspace"
)

const audioSpace = "clap-htsat-unfused@r1"
const audioShadow = "clap-test-shadow@r1"

func enableBackfillAudio(t *testing.T, ctx context.Context, s *Service) {
	t.Helper()
	metadata := mlclient.SpacesResponse{Spaces: []mlclient.SpaceInfo{}, Transcribers: []mlclient.TranscriberInfo{}}
	for _, id := range []string{"active@r1", "shadow@r1"} {
		info, _ := s.deps.Registry.Get(id)
		span := info.MaxSpanMs
		metadata.Spaces = append(metadata.Spaces, mlclient.SpaceInfo{ID: info.ID, Dim: info.Dim, Metric: info.Metric, Fingerprint: info.Fingerprint, DocInputs: info.DocInputs, QueryInputs: info.QueryInputs, MaxSpanMs: &span, MaxBatch: info.MaxBatch})
	}
	if model, _, ok := s.deps.Registry.Transcriber(); ok {
		metadata.Transcribers = append(metadata.Transcribers, model)
	}
	span := 10000
	// Synthetic discovery fixtures exercise the CLAP contract, not real-model retrieval quality.
	for _, id := range []string{audioSpace, audioShadow} {
		metadata.Spaces = append(metadata.Spaces, mlclient.SpaceInfo{ID: id, Dim: 512, Metric: "cosine", Fingerprint: "sha256:" + strings.Repeat("c", 64), DocInputs: []string{"audio"}, QueryInputs: []string{"text"}, MaxSpanMs: &span, MaxBatch: 32})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(metadata) }))
	t.Cleanup(server.Close)
	s.deps.Registry = space.New(s.deps.Pool, mlclient.New(config.MLConfig{Endpoints: []string{server.URL}, Token: "explicit-backfill-contract"}, nil), config.MLConfig{Transcriber: "asr@r1"})
	if err := s.deps.Registry.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	cfg := config.ChannelsConfig{}
	for _, channel := range s.deps.Channels.List() {
		cfg[channel.Name] = config.ChannelConfig{Doc: channel.Doc, Seg: channel.Seg, Weight: channel.Weight, Active: channel.Active, Shadow: channel.Shadow}
	}
	cfg["audio"] = config.ChannelConfig{Doc: "audio", Seg: timeline.GridSeg, Weight: 1, Active: []string{audioSpace}, Shadow: []string{audioShadow}}
	var err error
	s.deps.Channels, err = space.NewChannels(cfg)
	if err != nil {
		t.Fatal(err)
	}
}

// Seed a historical ready snapshot; reconciliation must preserve these already-published results.
func historicalReady(t *testing.T, ctx context.Context, s *Service, hasAudio bool) (int64, int64) {
	t.Helper()
	ws, id := uploadFixture(t, ctx, s.deps.Pool)
	if err := library.UpdateProbe(ctx, s.deps.Pool, id, 90000, true, hasAudio); err != nil {
		t.Fatal(err)
	}
	if err := library.UpdatePreparedMedia(ctx, s.deps.Pool, id, "published/normalized_video.mp4", timeline.SpriteLayout{TileW: 320, TileH: 180, Cols: 10, Rows: 10, Count: 36}); err != nil {
		t.Fatal(err)
	}
	video, err := library.GetForWorker(ctx, s.deps.Pool, id)
	if err != nil {
		t.Fatal(err)
	}
	oldChannels := slices.DeleteFunc(s.deps.Channels.List(), func(channel space.Channel) bool { return channel.Name == "audio" })
	plan, err := ExpandPlan(video, oldChannels, s.deps.Registry)
	if err != nil {
		t.Fatal(err)
	}
	err = db.WithTx(ctx, s.deps.Pool, func(tx pgx.Tx) error {
		if err := addPlan(ctx, tx, video, append(InitialPlan(video), plan...)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE stage_run SET status='done',attempt=1 WHERE video_id=$1", id); err != nil {
			return err
		}
		if hasAudio {
			if err := index.ReplaceSpeech(ctx, tx, ws, id, "historical-asr", []index.TextSpan{{Span: timeline.Span{StartMs: 0, EndMs: 1000}, Text: "必须保留的中文原文"}}); err != nil {
				return err
			}
		}
		if err := index.UpsertVectors(ctx, tx, []index.VectorRow{{VideoID: id, WorkspaceID: ws, Channel: "visual", SpaceID: "active@r1", Seg: timeline.GridSeg, Span: timeline.Span{StartMs: 0, EndMs: 2500}, Vec: []float32{1, 0, 0}}}); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO usage_event(workspace_id,kind,amount,ref) VALUES($1,'indexed_ms',90000,'historical')", ws); err != nil {
			return err
		}
		return library.SetStatus(ctx, tx, id, "ready", "")
	})
	if err != nil {
		t.Fatal(err)
	}
	return ws, id
}

func TestReadyAudioBackfillAndRetryIntegration(t *testing.T) {
	ctx, s := pipelineFixture(t)
	ws, id := historicalReady(t, ctx, s, true)
	before, err := library.GetForWorker(ctx, s.deps.Pool, id)
	if err != nil {
		t.Fatal(err)
	}
	old, err := pipelinedb.New(s.deps.Pool).Stages(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	enableBackfillAudio(t, ctx, s)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := s.Reconcile(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	rows, err := pipelinedb.New(s.deps.Pool).Stages(ctx, id)
	if err != nil || len(rows) != len(old)+2 {
		t.Fatalf("concurrent backfill duplicated stages: %d old=%d %v", len(rows), len(old), err)
	}
	for _, row := range old {
		current := onlyStage(t, ctx, s, id, row.Stage)
		if !reflect.DeepEqual(current, row) {
			t.Fatalf("historical stage changed: %s", row.Stage)
		}
		_, p := taskForRow(t, ws, row)
		if _, err := s.deps.Inspector.GetTaskInfo(decodeQueue(t, row.Output), queue.StageTaskID(p)); !errors.Is(err, asynq.ErrTaskNotFound) && !errors.Is(err, asynq.ErrQueueNotFound) {
			t.Fatalf("historical stage requeued: %s %v", row.Stage, err)
		}
	}
	for _, spaceID := range []string{audioSpace, audioShadow} {
		row := onlyStage(t, ctx, s, id, "embed:audio:"+spaceID)
		output, err := decodeOutput(row.Output)
		if err != nil || row.Required || row.Status != "queued" || output.Plan.Required || output.Plan.Queue != queue.QueueGPULow || !slices.Equal(output.Plan.Deps, []string{"prepare_media"}) || output.Plan.Options.Space.ID != spaceID || output.Plan.Options.Space.Dim != 512 {
			t.Fatalf("invalid optional audio plan: %+v %+v %v", row, output.Plan, err)
		}
		_, p := taskForRow(t, ws, row)
		if _, err := s.deps.Inspector.GetTaskInfo(queue.QueueGPULow, queue.StageTaskID(p)); err != nil {
			t.Fatalf("audio not delivered to gpu_low: %v", err)
		}
	}
	checkPreserved := func() {
		t.Helper()
		current, err := library.GetForWorker(ctx, s.deps.Pool, id)
		if err != nil || !reflect.DeepEqual(current, before) {
			t.Fatalf("ready metadata changed: %+v %v", current, err)
		}
		var text string
		var vectors, usage int
		if err := s.deps.Pool.QueryRow(ctx, "SELECT (SELECT text FROM text_span WHERE video_id=$1),(SELECT count(*) FROM embedding WHERE video_id=$1 AND channel='visual'),(SELECT count(*) FROM usage_event WHERE workspace_id=$2)", id, ws).Scan(&text, &vectors, &usage); err != nil || text != "必须保留的中文原文" || vectors != 1 || usage != 1 {
			t.Fatalf("historical output changed: %q %d %d %v", text, vectors, usage, err)
		}
	}
	checkPreserved()
	if err := s.Retry(ctx, ws, id); apperr.KindOf(err) != apperr.Conflict {
		t.Fatalf("retried ready video without failed optional stage: %v", err)
	}
	failed := onlyStage(t, ctx, s, id, "embed:audio:"+audioSpace)
	task, payload := taskForRow(t, ws, failed)
	s.deps.Runs["embed"] = func(ctx context.Context, sc *StageCtx) (Commit, error) {
		if err := sc.SaveProgress(ctx, map[string]int{"next": 1, "total": 36}, nil); err != nil {
			return nil, err
		}
		return nil, apperr.New(apperr.Invalid, "invalid_audio", "explicit optional backfill failure")
	}
	if err := s.HandleTask(ctx, task); !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("optional failure not terminal: %v", err)
	}
	failed = onlyStage(t, ctx, s, id, failed.Stage)
	if failed.Status != "failed" {
		t.Fatalf("optional stage not failed: %+v", failed)
	}
	checkPreserved()
	if err := s.Retry(ctx, ws+1, id); apperr.KindOf(err) != apperr.NotFound {
		t.Fatalf("foreign retry: %v", err)
	}
	if err := s.Retry(ctx, ws, id); err != nil {
		t.Fatal(err)
	}
	retried := onlyStage(t, ctx, s, id, failed.Stage)
	if retried.Version == failed.Version || retried.Status != "queued" || retried.Attempt != 0 || !reflect.DeepEqual(retried.Output, failed.Output) {
		t.Fatalf("optional retry lost fencing/checkpoint: old=%+v new=%+v", failed, retried)
	}
	if _, err := s.claim(ctx, payload); !errors.Is(err, ErrStale) {
		t.Fatalf("old optional generation still claimable: %v", err)
	}
	checkPreserved()
	var runs int
	s.deps.Runs["embed"] = func(_ context.Context, sc *StageCtx) (Commit, error) {
		runs++
		vec := make([]float32, 512)
		vec[0] = 1
		return func(ctx context.Context, tx pgx.Tx) error {
			return index.UpsertVectors(ctx, tx, []index.VectorRow{{VideoID: id, WorkspaceID: ws, Channel: "audio", SpaceID: sc.Def.Options.Space.ID, Seg: timeline.GridSeg, Span: timeline.Span{StartMs: 0, EndMs: 2500}, Vec: vec}})
		}, nil
	}
	if err := s.HandleTask(ctx, task); err != nil || runs != 0 {
		t.Fatalf("old queued delivery reran encoding: runs=%d %v", runs, err)
	}
	currentTask, _ := taskForRow(t, ws, retried)
	if err := s.HandleTask(ctx, currentTask); err != nil || runs != 1 {
		t.Fatalf("retried optional task did not commit its new vectors: runs=%d %v", runs, err)
	}
	var audioCount, dim int
	if err := s.deps.Pool.QueryRow(ctx, "SELECT count(*),max(vector_dims(vec)) FROM embedding WHERE video_id=$1 AND channel='audio' AND space_id=$2", id, audioSpace).Scan(&audioCount, &dim); err != nil || audioCount != 1 || dim != 512 {
		t.Fatalf("new audio embedding absent: count=%d dim=%d %v", audioCount, dim, err)
	}
	for _, row := range old {
		if current := onlyStage(t, ctx, s, id, row.Stage); !reflect.DeepEqual(current, row) {
			t.Fatalf("backfill retry/success changed historical stage: %s", row.Stage)
		}
	}
	if err := s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	checkPreserved()
}

func decodeQueue(t *testing.T, data []byte) string {
	t.Helper()
	output, err := decodeOutput(data)
	if err != nil {
		t.Fatal(err)
	}
	return output.Plan.Queue
}

func TestBackfillEligibilityAndDeleteFenceIntegration(t *testing.T) {
	ctx, s := pipelineFixture(t)
	enableBackfillAudio(t, ctx, s)
	_, silent := historicalReady(t, ctx, s, false)
	_, deleted := historicalReady(t, ctx, s, true)
	_, indexing := historicalReady(t, ctx, s, true)
	_, unprepared := historicalReady(t, ctx, s, true)
	_, missingDep := historicalReady(t, ctx, s, true)
	if _, err := s.deps.Pool.Exec(ctx, "UPDATE video SET deleted_at=now() WHERE id=$1", deleted); err != nil {
		t.Fatal(err)
	}
	if err := library.SetStatus(ctx, s.deps.Pool, indexing, "indexing", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.deps.Pool.Exec(ctx, "UPDATE video SET normalized_video_key=NULL WHERE id=$1", unprepared); err != nil {
		t.Fatal(err)
	}
	if _, err := s.deps.Pool.Exec(ctx, "DELETE FROM stage_run WHERE video_id=$1 AND stage='prepare_media'", missingDep); err != nil {
		t.Fatal(err)
	}
	if err := s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{silent, deleted, indexing, unprepared, missingDep} {
		var count int
		if err := s.deps.Pool.QueryRow(ctx, "SELECT count(*) FROM stage_run WHERE video_id=$1 AND stage LIKE 'embed:audio:%'", id).Scan(&count); err != nil || count != 0 {
			t.Fatalf("ineligible video %d got audio stage: %d %v", id, count, err)
		}
	}
	ws, id := historicalReady(t, ctx, s, true)
	targets := s.backfillTargets()
	lb := library.New(s.deps.Pool, nil, library.Config{}, s, s)
	if err := lb.Delete(ctx, workspace.Principal{WorkspaceID: ws}, id); err != nil {
		t.Fatal(err)
	}
	if err := s.backfillVideo(ctx, id, targets); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.deps.Pool.QueryRow(ctx, "SELECT count(*) FROM stage_run WHERE video_id=$1", id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale candidate resurrected deleted stages: %d %v", count, err)
	}
}

func TestBackfillBatchBoundIntegration(t *testing.T) {
	ctx, s := pipelineFixture(t)
	enableBackfillAudio(t, ctx, s)
	var err error
	s.deps.Channels, err = space.NewChannels(config.ChannelsConfig{"audio": {Doc: "audio", Seg: timeline.GridSeg, Weight: 1, Active: []string{audioSpace}}})
	if err != nil {
		t.Fatal(err)
	}
	ws, _ := uploadFixture(t, ctx, s.deps.Pool)
	output, _ := json.Marshal(stageOutput{Plan: StageDef{Name: "prepare_media", Version: "v1", Queue: queue.QueueCPU, Required: true, Timeout: time.Hour}})
	_, err = s.deps.Pool.Exec(ctx, `WITH videos AS (
INSERT INTO video(workspace_id,title,source_kind,status,duration_ms,has_video,has_audio,normalized_video_key)
SELECT $1,'historical bound fixture','upload','ready',1000,false,true,'published/normalized_video.mp4' FROM generate_series(1,101) RETURNING id)
INSERT INTO stage_run(video_id,stage,version,status,required,output) SELECT id,'prepare_media','historical','done',true,$2 FROM videos`, ws, output)
	if err != nil {
		t.Fatal(err)
	}
	// Ignore the unrelated uploading fixture; this test bounds only ready embedding backfill.
	if _, err := s.deps.Pool.Exec(ctx, "UPDATE video SET source_key=NULL WHERE status='uploading'"); err != nil {
		t.Fatal(err)
	}
	if err := s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.deps.Pool.QueryRow(ctx, "SELECT count(*) FROM stage_run WHERE stage=$1", "embed:audio:"+audioSpace).Scan(&count); err != nil || count != 100 {
		t.Fatalf("first backfill batch=%d %v", count, err)
	}
	if err := s.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.deps.Pool.QueryRow(ctx, "SELECT count(*) FROM stage_run WHERE stage=$1", "embed:audio:"+audioSpace).Scan(&count); err != nil || count != 101 {
		t.Fatalf("second backfill batch=%d %v", count, err)
	}
}
