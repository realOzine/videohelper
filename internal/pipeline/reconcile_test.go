package pipeline

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/pipeline/pipelinedb"
	"github.com/ozine/videohelper/internal/platform/queue"
	"github.com/redis/go-redis/v9"
)

func TestReconcileLostCrashedAndUnstartedIntegration(t *testing.T) {
	ctx, service := pipelineFixture(t)
	workspaceID, videoID := uploadFixture(t, ctx, service.deps.Pool)
	if err := service.Start(ctx, videoID); err != nil {
		t.Fatal(err)
	}
	row := onlyStage(t, ctx, service, videoID, "probe")
	_, p := taskForRow(t, workspaceID, row)
	if err := service.deps.Inspector.DeleteTask(queue.QueueCPU, queue.StageTaskID(p)); err != nil {
		t.Fatal(err)
	}
	age := func(videoID int64) {
		t.Helper()
		if _, err := service.deps.Pool.Exec(ctx, "UPDATE stage_run SET updated_at=now()-interval '1 hour' WHERE video_id=$1", videoID); err != nil {
			t.Fatal(err)
		}
	}
	age(videoID)
	if err := service.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := service.deps.Inspector.GetTaskInfo(queue.QueueCPU, queue.StageTaskID(p)); err != nil {
		t.Fatalf("lost queued task not recovered: %v", err)
	}
	if _, err := service.claim(ctx, p); err != nil {
		t.Fatal(err)
	}
	age(videoID)
	if err := service.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	next := onlyStage(t, ctx, service, videoID, "probe")
	if next.Version == row.Version || next.Status != "queued" {
		t.Fatalf("crashed generation not retired: old=%+v next=%+v", row, next)
	}
	if _, err := service.deps.Inspector.GetTaskInfo(queue.QueueCPU, queue.StageTaskID(p)); !errors.Is(err, asynq.ErrTaskNotFound) {
		t.Fatalf("old queued Asynq task not removed: %v", err)
	}
	_, p = taskForRow(t, workspaceID, next)
	claimed, err := service.claim(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	age(videoID)
	now := time.Now()
	snapshots, err := pipelinedb.New(service.deps.Pool).ReconcileStages(ctx, pipelinedb.ReconcileStagesParams{
		BeforeTime: pgtype.Timestamptz{Time: now.Add(-10 * time.Minute), Valid: true}, NowTime: pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("reconcile snapshots=%v error=%v", snapshots, err)
	}
	if err := service.saveProgress(ctx, p, claimed.Attempt, map[string]int{"fresh": 1}, nil); err != nil {
		t.Fatal(err)
	}
	if err := service.recoverStage(ctx, snapshots[0]); err != nil {
		t.Fatal(err)
	}
	current := onlyStage(t, ctx, service, videoID, "probe")
	if current.Version != next.Version || current.Status != "running" {
		t.Fatalf("reconciler stole refreshed attempt: %+v", current)
	}
	otherWorkspace, otherVideo := uploadFixture(t, ctx, service.deps.Pool)
	if err := service.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	other := onlyStage(t, ctx, service, otherVideo, "probe")
	if other.Status != "queued" {
		t.Fatalf("uploaded source not started: %+v", other)
	}
	_, otherPayload := taskForRow(t, otherWorkspace, other)
	if err := service.deps.Inspector.ArchiveTask(queue.QueueCPU, queue.StageTaskID(otherPayload)); err != nil {
		t.Fatal(err)
	}
	age(otherVideo)
	if err := service.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if recovered := onlyStage(t, ctx, service, otherVideo, "probe"); recovered.Version == other.Version || recovered.Status != "queued" {
		t.Fatalf("stranded archived task not recovered: %+v", recovered)
	}
}

func TestRegisteredWorkerThrottleBudgetIntegration(t *testing.T) {
	for _, budget := range []int{0, 1} {
		t.Run(fmt.Sprint(budget), func(t *testing.T) {
			ctx, service := pipelineFixture(t)
			service.deps.Worker.MaxRetry = budget
			workspaceID, videoID := uploadFixture(t, ctx, service.deps.Pool)
			var calls atomic.Int32
			started := make(chan struct{})
			release := make(chan struct{})
			releaseOnce := sync.OnceFunc(func() { close(release) })
			service.deps.Runs["probe"] = func(ctx context.Context, sc *StageCtx) (Commit, error) {
				n := int(calls.Add(1))
				if budget == 1 && n == 1 {
					return nil, errors.New("temporary failure before the last retry")
				}
				if n == budget+1 {
					return nil, &mlclient.Error{Status: 503, Retryable: true}
				}
				if n != budget+2 {
					return nil, errors.New("unexpected extra execution")
				}
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				if budget == 1 {
					return nil, errors.New("real failure on the last retry")
				}
				return func(ctx context.Context, tx pgx.Tx) error {
					return library.UpdateProbe(ctx, tx, sc.Video.ID, 30, true, false)
				}, nil
			}
			server := asynq.NewServer(asynq.RedisClientOpt{Addr: service.deps.Redis.(*redis.Client).Options().Addr, DB: 14}, asynq.Config{
				Concurrency: 1, Queues: map[string]int{queue.QueueCPU: 1}, ShutdownTimeout: time.Second,
				TaskCheckInterval: 10 * time.Millisecond, DelayedTaskCheckInterval: 10 * time.Millisecond,
				RetryDelayFunc: func(int, error, *asynq.Task) time.Duration { return 10 * time.Millisecond }, IsFailure: queue.IsFailure,
			})
			mux := asynq.NewServeMux()
			service.Register(mux)
			if err := server.Start(mux); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(server.Shutdown)
			t.Cleanup(releaseOnce)
			if err := service.Start(ctx, videoID); err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(10 * time.Second):
				t.Fatal("throttling exhausted Asynq's retry budget")
			}
			row := onlyStage(t, ctx, service, videoID, "probe")
			_, p := taskForRow(t, workspaceID, row)
			info, err := service.deps.Inspector.GetTaskInfo(queue.QueueCPU, queue.StageTaskID(p))
			if err != nil || info.State != asynq.TaskStateActive || info.Retried != budget || info.MaxRetry != budget+1 || int(calls.Load()) != budget+2 {
				t.Fatalf("retry accounting info=%+v calls=%d error=%v", info, calls.Load(), err)
			}
			if _, err := service.deps.Pool.Exec(ctx, "UPDATE stage_run SET updated_at=now()-interval '1 hour' WHERE video_id=$1", videoID); err != nil {
				t.Fatal(err)
			}
			if err := service.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}
			if current := onlyStage(t, ctx, service, videoID, "probe"); current.Version != row.Version || current.Attempt != row.Attempt || current.Status != "running" {
				t.Fatalf("reconciler stole active Asynq task: %+v", current)
			}
			server.Stop()
			releaseOnce()
			deadline := time.After(5 * time.Second)
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for {
				status := onlyStage(t, ctx, service, videoID, "probe").Status
				if budget == 0 && status == "done" {
					break
				}
				if budget == 1 && status == "failed" {
					info, err := service.deps.Inspector.GetTaskInfo(queue.QueueCPU, queue.StageTaskID(p))
					if err == nil && info.State == asynq.TaskStateArchived && info.Retried == budget {
						break
					}
				}
				select {
				case <-deadline:
					t.Fatal("registered worker did not enforce the real retry budget")
				case <-ticker.C:
				}
			}
		})
	}
}

func TestGPUSharedLeaseLifetimeIntegration(t *testing.T) {
	ctx, service := pipelineFixture(t)
	workspaceID, _ := uploadFixture(t, ctx, service.deps.Pool)
	long := StageDef{Queue: queue.QueueGPU, Timeout: 10 * time.Minute}
	first, err := service.acquireGPU(ctx, workspaceID, long)
	if err != nil {
		t.Fatal(err)
	}
	otherWorker := New(service.deps)
	if _, err := otherWorker.acquireGPU(ctx, workspaceID, long); !errors.Is(err, queue.ErrThrottled) {
		t.Fatalf("another worker exceeded workspace limit: %v", err)
	}
	first()
	service.deps.Worker.WorkspaceGPULimit = 2
	first, err = service.acquireGPU(ctx, workspaceID, long)
	if err != nil {
		t.Fatal(err)
	}
	defer first()
	second, err := service.acquireGPU(ctx, workspaceID, StageDef{Queue: queue.QueueGPU, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer second()
	key := fmt.Sprintf("vh:gpu:%d", workspaceID)
	if ttl, err := service.deps.Redis.PTTL(ctx, key).Result(); err != nil || ttl < 10*time.Minute {
		t.Fatalf("short lease shortened longer worker's expiry: ttl=%v error=%v", ttl, err)
	}
}
