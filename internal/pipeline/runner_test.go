package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/pipeline/pipelinedb"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/platform/queue"
	"github.com/ozine/videohelper/internal/timeline"
)

func taskForRow(t *testing.T, workspaceID int64, row pipelinedb.StageRun) (*asynq.Task, queue.StagePayload) {
	t.Helper()
	p := queue.StagePayload{WorkspaceID: workspaceID, VideoID: row.VideoID, Stage: row.Stage, Version: row.Version}
	body, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return asynq.NewTask(queue.TypeStage, body), p
}

// Synthetic callbacks exercise orchestration and transactional outputs, not media/model acceptance.
func TestRunnerOrchestrationIntegration(t *testing.T) {
	ctx, service := pipelineFixture(t)
	workspaceID, videoID := uploadFixture(t, ctx, service.deps.Pool)
	service.deps.Runs = map[string]RunFunc{
		"probe": func(ctx context.Context, sc *StageCtx) (Commit, error) {
			// A second connection can lock the video during Run: external work holds no DB transaction.
			err := db.WithTx(ctx, service.deps.Pool, func(tx pgx.Tx) error {
				_, err := tx.Exec(ctx, "SELECT id FROM video WHERE id=$1 FOR UPDATE NOWAIT", sc.Video.ID)
				return err
			})
			return func(ctx context.Context, tx pgx.Tx) error {
				return library.UpdateProbe(ctx, tx, sc.Video.ID, 30, true, true)
			}, err
		},
		"prepare_media": func(context.Context, *StageCtx) (Commit, error) { return nil, nil },
		"asr": func(ctx context.Context, sc *StageCtx) (Commit, error) {
			err := sc.SaveProgress(ctx, map[string]int{"chunk": 1}, func(ctx context.Context, tx pgx.Tx) error {
				return index.ReplaceSpeech(ctx, tx, sc.Video.WorkspaceID, sc.Video.ID, "test-asr", []index.TextSpan{{Span: timeline.Span{StartMs: 0, EndMs: 30}, Text: "测试句子"}})
			})
			return nil, err
		},
		"embed": func(ctx context.Context, sc *StageCtx) (Commit, error) {
			if sc.Def.Options.Space.Fingerprint != "sha256:"+strings.Repeat("a", 64) {
				t.Error("embedding metadata snapshot was lost")
			}
			return func(ctx context.Context, tx pgx.Tx) error {
				return index.UpsertVectors(ctx, tx, []index.VectorRow{{VideoID: sc.Video.ID, WorkspaceID: sc.Video.WorkspaceID,
					Channel: sc.Def.Options.Channel, SpaceID: sc.Def.Options.Space.ID, Seg: sc.Def.Options.Seg,
					Span: timeline.Span{StartMs: 0, EndMs: 30}, Vec: []float32{1, 0, 0}}})
			}, nil
		},
		"finalize": func(ctx context.Context, sc *StageCtx) (Commit, error) {
			return func(ctx context.Context, tx pgx.Tx) error {
				if err := library.SetStatus(ctx, tx, sc.Video.ID, "ready", ""); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, "INSERT INTO usage_event(workspace_id,kind,amount,ref) VALUES($1,'indexed_ms',30,'test')", sc.Video.WorkspaceID)
				return err
			}, nil
		},
	}
	if err := service.Start(ctx, videoID); err != nil {
		t.Fatal(err)
	}
	for range 10 {
		rows, err := pipelinedb.New(service.deps.Pool).Stages(ctx, videoID)
		if err != nil {
			t.Fatal(err)
		}
		var ran bool
		for _, row := range rows {
			output, err := decodeOutput(row.Output)
			if err != nil {
				t.Fatal(err)
			}
			if row.Status != "queued" || !row.Required {
				continue
			}
			task, _ := taskForRow(t, workspaceID, row)
			if err := service.HandleTask(ctx, task); err != nil {
				t.Fatalf("stage %s: %v", row.Stage, err)
			}
			if output.Plan.Name == "probe" {
				// Later configuration changes cannot alter the already persisted graph.
				service.deps.Channels, service.deps.Registry = nil, nil
			}
			ran = true
		}
		if !ran {
			break
		}
	}
	var status string
	if err := service.deps.Pool.QueryRow(ctx, "SELECT status FROM video WHERE id=$1", videoID).Scan(&status); err != nil || status != "ready" {
		t.Fatalf("video status=%s error=%v", status, err)
	}
	shadow := onlyStage(t, ctx, service, videoID, "embed:visual:shadow@r1")
	if shadow.Status != "queued" || shadow.Required {
		t.Fatalf("shadow unexpectedly blocked readiness: %+v", shadow)
	}
	finalize := onlyStage(t, ctx, service, videoID, "finalize")
	task, _ := taskForRow(t, workspaceID, finalize)
	if err := service.HandleTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := service.deps.Pool.QueryRow(ctx, "SELECT count(*) FROM usage_event WHERE workspace_id=$1", workspaceID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("idempotent finalization count=%d error=%v", count, err)
	}
	asr := onlyStage(t, ctx, service, videoID, "asr")
	output, err := decodeOutput(asr.Output)
	if err != nil || progressValue(t, output.Progress, "chunk") != 1 {
		t.Fatalf("ASR checkpoint=%s error=%v", output.Progress, err)
	}
}

func TestAttemptAndGenerationFencesIntegration(t *testing.T) {
	ctx, service := pipelineFixture(t)
	workspaceID, videoID := uploadFixture(t, ctx, service.deps.Pool)
	if err := service.Start(ctx, videoID); err != nil {
		t.Fatal(err)
	}
	row := onlyStage(t, ctx, service, videoID, "probe")
	_, p := taskForRow(t, workspaceID, row)
	first, err := service.claim(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint := func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "UPDATE video SET title='checkpoint' WHERE id=$1", videoID)
		return err
	}
	if err := service.saveProgress(ctx, p, first.Attempt, map[string]int{"batch": 1}, checkpoint); err != nil {
		t.Fatal(err)
	}
	checkpointErr := errors.New("checkpoint write failed")
	err = service.saveProgress(ctx, p, first.Attempt, map[string]int{"batch": 2}, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "UPDATE video SET title='must rollback' WHERE id=$1", videoID); err != nil {
			return err
		}
		return checkpointErr
	})
	if !errors.Is(err, checkpointErr) {
		t.Fatalf("checkpoint failure=%v", err)
	}
	var title string
	if err := service.deps.Pool.QueryRow(ctx, "SELECT title FROM video WHERE id=$1", videoID).Scan(&title); err != nil || title != "checkpoint" {
		t.Fatalf("checkpoint output did not roll back: title=%s error=%v", title, err)
	}
	if _, err := service.deps.Pool.Exec(ctx, "UPDATE stage_run SET status='queued' WHERE video_id=$1", videoID); err != nil {
		t.Fatal(err)
	}
	second, err := service.claim(ctx, p)
	if err != nil || second.Attempt != first.Attempt+1 || progressValue(t, second.Progress, "batch") != 1 {
		t.Fatalf("resumed attempt=%+v error=%v", second, err)
	}
	called := false
	staleWrite := func(context.Context, pgx.Tx) error { called = true; return nil }
	if err := service.saveProgress(ctx, p, first.Attempt, nil, staleWrite); !errors.Is(err, ErrStale) {
		t.Fatalf("old attempt progress=%v", err)
	}
	if _, err := service.complete(ctx, p, first.Attempt, staleWrite); !errors.Is(err, ErrStale) || called {
		t.Fatalf("old attempt commit=%v callback=%v", err, called)
	}
	if err := service.fail(ctx, p, second.Attempt, errors.Join(asynq.SkipRetry, errors.New("terminal"))); !errors.Is(err, asynq.SkipRetry) {
		t.Fatal(err)
	}
	if err := service.Retry(ctx, workspaceID, videoID); err != nil {
		t.Fatal(err)
	}
	if err := service.saveProgress(ctx, p, second.Attempt, nil, staleWrite); !errors.Is(err, ErrStale) || called {
		t.Fatalf("retired generation progress=%v callback=%v", err, called)
	}
	if _, err := service.complete(ctx, p, second.Attempt, staleWrite); !errors.Is(err, ErrStale) || called {
		t.Fatalf("retired generation commit=%v callback=%v", err, called)
	}
	newRow := onlyStage(t, ctx, service, videoID, "probe")
	_, next := taskForRow(t, workspaceID, newRow)
	third, err := service.claim(ctx, next)
	if err != nil {
		t.Fatal(err)
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
	if err := service.saveProgress(ctx, next, third.Attempt, nil, staleWrite); !errors.Is(err, ErrStale) || called {
		t.Fatalf("deleted/restored generation progress=%v callback=%v", err, called)
	}
	if _, err := service.complete(ctx, next, third.Attempt, staleWrite); !errors.Is(err, ErrStale) || called {
		t.Fatalf("deleted/restored generation commit=%v callback=%v", err, called)
	}
}

func TestCommitRollbackAndCancellationIntegration(t *testing.T) {
	ctx, service := pipelineFixture(t)
	workspaceID, videoID := uploadFixture(t, ctx, service.deps.Pool)
	if err := service.Start(ctx, videoID); err != nil {
		t.Fatal(err)
	}
	task, _ := taskForRow(t, workspaceID, onlyStage(t, ctx, service, videoID, "probe"))
	writeErr := errors.New("output commit failed")
	service.deps.Runs["probe"] = func(context.Context, *StageCtx) (Commit, error) {
		return func(ctx context.Context, tx pgx.Tx) error {
			if err := library.UpdateProbe(ctx, tx, videoID, 30, true, false); err != nil {
				return err
			}
			return writeErr
		}, nil
	}
	if err := service.HandleTask(ctx, task); !errors.Is(err, writeErr) {
		t.Fatal(err)
	}
	var hasDuration bool
	if err := service.deps.Pool.QueryRow(ctx, "SELECT duration_ms IS NOT NULL FROM video WHERE id=$1", videoID).Scan(&hasDuration); err != nil || hasDuration {
		t.Fatalf("failed output commit leaked metadata: %v error=%v", hasDuration, err)
	}
	if row := onlyStage(t, ctx, service, videoID, "probe"); row.Status != "queued" {
		t.Fatalf("failed output commit finished stage: %+v", row)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	committed := false
	service.deps.Runs["probe"] = func(_ context.Context, sc *StageCtx) (Commit, error) {
		cancel()
		if err := sc.SaveProgress(context.Background(), nil, func(context.Context, pgx.Tx) error { committed = true; return nil }); !errors.Is(err, context.Canceled) {
			t.Errorf("canceled stage checkpoint=%v", err)
		}
		return func(context.Context, pgx.Tx) error { committed = true; return nil }, nil
	}
	if err := service.HandleTask(runCtx, task); !errors.Is(err, context.Canceled) || committed {
		t.Fatalf("canceled stage error=%v committed=%v", err, committed)
	}
	if row := onlyStage(t, ctx, service, videoID, "probe"); row.Status != "queued" {
		t.Fatalf("canceled stage did not persist retryable state: %+v", row)
	}
}

func progressValue(t *testing.T, data json.RawMessage, key string) int {
	t.Helper()
	var value map[string]int
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value[key]
}

func TestGPUThrottleAndPermanentFailureIntegration(t *testing.T) {
	ctx, service := pipelineFixture(t)
	workspaceID, videoID := uploadFixture(t, ctx, service.deps.Pool)
	if err := service.Start(ctx, videoID); err != nil {
		t.Fatal(err)
	}
	def := StageDef{Name: "asr", Version: "v1", Queue: queue.QueueGPU, Timeout: time.Minute, Required: true}
	if err := db.WithTx(ctx, service.deps.Pool, func(tx pgx.Tx) error {
		video, err := library.GetForWorker(ctx, tx, videoID)
		if err != nil {
			return err
		}
		if err := addPlan(ctx, tx, video, []StageDef{def}); err != nil {
			return err
		}
		_, err = advance(ctx, tx, video)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	release, err := service.acquireGPU(ctx, workspaceID, def)
	if err != nil {
		t.Fatal(err)
	}
	row := onlyStage(t, ctx, service, videoID, "asr")
	task, _ := taskForRow(t, workspaceID, row)
	service.deps.Runs["asr"] = func(context.Context, *StageCtx) (Commit, error) {
		t.Error("throttled stage executed")
		return nil, nil
	}
	if err := service.HandleTask(ctx, task); !errors.Is(err, queue.ErrThrottled) || queue.IsFailure(err) {
		t.Fatalf("workspace throttle=%v", err)
	}
	if row := onlyStage(t, ctx, service, videoID, "asr"); row.Attempt != 0 || row.Status != "queued" {
		t.Fatalf("throttling claimed stage: %+v", row)
	}
	release()
	service.deps.Worker.MaxRetry = 0
	for _, status := range []int{503, 504} {
		service.deps.Runs["asr"] = func(context.Context, *StageCtx) (Commit, error) {
			return nil, &mlclient.Error{Status: status, Retryable: true}
		}
		if err := service.HandleTask(ctx, task); !errors.Is(err, queue.ErrThrottled) || queue.IsFailure(err) {
			t.Fatalf("ML status=%d throttle=%v", status, err)
		}
		if row := onlyStage(t, ctx, service, videoID, "asr"); row.Status != "queued" {
			t.Fatalf("ML throttling exhausted retry budget: %+v", row)
		}
	}
	service.deps.Runs["asr"] = func(context.Context, *StageCtx) (Commit, error) {
		return nil, &mlclient.Error{Status: 422, Retryable: false}
	}
	if err := service.HandleTask(ctx, task); !errors.Is(err, asynq.SkipRetry) {
		t.Fatalf("permanent failure=%v", err)
	}
	if row := onlyStage(t, ctx, service, videoID, "asr"); row.Status != "failed" {
		t.Fatalf("permanent stage=%+v", row)
	}
	anotherWorkspace, another := uploadFixture(t, ctx, service.deps.Pool)
	if err := service.Start(ctx, another); err != nil {
		t.Fatal(err)
	}
	missing, _ := taskForRow(t, anotherWorkspace, onlyStage(t, ctx, service, another, "probe"))
	if err := service.HandleTask(ctx, missing); !errors.Is(err, asynq.SkipRetry) || apperr.KindOf(err) != apperr.Internal {
		t.Fatalf("missing runner=%v", err)
	}
}
