package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/pipeline/pipelinedb"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/platform/queue"
	"github.com/redis/go-redis/v9"
)

func (s *Service) Register(mux *asynq.ServeMux) {
	mux.HandleFunc(queue.TypeStage, s.HandleTask)
	mux.HandleFunc(queue.TypeReconcile, func(ctx context.Context, _ *asynq.Task) error { return s.Reconcile(ctx) })
}

func (s *Service) HandleTask(ctx context.Context, task *asynq.Task) error {
	var payload queue.StagePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return errors.Join(asynq.SkipRetry, errors.New("invalid stage payload"))
	}
	if err := payload.Validate(); err != nil {
		return errors.Join(asynq.SkipRetry, err)
	}
	row, err := pipelinedb.New(s.deps.Pool).ReadStage(ctx, pipelinedb.ReadStageParams{
		VideoID: payload.VideoID, Stage: payload.Stage, Version: payload.Version, WorkspaceID: payload.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.Status != "queued") {
		return nil
	}
	if err != nil {
		return err
	}
	output, err := decodeOutput(row.Output)
	if err != nil {
		return err
	}
	release, err := s.acquireGPU(ctx, payload.WorkspaceID, output.Plan)
	if err != nil {
		return err
	}
	defer release()
	sc, err := s.claim(ctx, payload)
	if errors.Is(err, ErrStale) {
		return nil
	}
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithTimeout(ctx, sc.Def.Timeout)
	defer cancel()
	sc.SaveProgress = func(ctx context.Context, v any, commit Commit) error {
		if err := runCtx.Err(); err != nil {
			return err
		}
		checkpoint, cancel := context.WithCancel(ctx)
		stop := context.AfterFunc(runCtx, cancel)
		defer func() { stop(); cancel() }()
		return s.saveProgress(checkpoint, payload, sc.Attempt, v, commit)
	}
	key := strings.SplitN(sc.Def.Name, ":", 2)[0]
	run := s.deps.Runs[key]
	var commit Commit
	if run == nil {
		err = errors.Join(asynq.SkipRetry, apperr.New(apperr.Internal, "stage_unavailable", "阶段处理器尚未配置"))
	} else {
		commit, err = run(runCtx, sc)
	}
	if err == nil {
		err = runCtx.Err()
	}
	if err == nil {
		var tasks []stageTask
		tasks, err = s.complete(runCtx, payload, sc.Attempt, commit)
		if err == nil {
			return s.enqueue(ctx, tasks)
		}
	}
	if errors.Is(err, ErrStale) {
		return nil
	}
	return s.fail(ctx, payload, sc.Attempt, err)
}

func (s *Service) claim(ctx context.Context, p queue.StagePayload) (*StageCtx, error) {
	var sc *StageCtx
	err := db.WithTx(ctx, s.deps.Pool, func(tx pgx.Tx) error {
		video, row, output, err := lockedStage(ctx, tx, p)
		if err != nil {
			return err
		}
		if row.Status != "queued" {
			return ErrStale
		}
		attempt, err := pipelinedb.New(tx).ClaimStage(ctx, pipelinedb.ClaimStageParams{VideoID: p.VideoID, Stage: p.Stage, Version: p.Version})
		if err != nil {
			return err
		}
		sc = &StageCtx{Video: video, Payload: p, Def: output.Plan, Attempt: int(attempt), Progress: output.Progress}
		return nil
	})
	return sc, err
}

func lockedStage(ctx context.Context, tx pgx.Tx, p queue.StagePayload) (library.Video, pipelinedb.StageRun, stageOutput, error) {
	video, err := library.GetForWorker(ctx, tx, p.VideoID)
	if apperr.KindOf(err) == apperr.NotFound || (err == nil && (video.WorkspaceID != p.WorkspaceID || (video.Status != "indexing" && video.Status != "ready"))) {
		return video, pipelinedb.StageRun{}, stageOutput{}, ErrStale
	}
	if err != nil {
		return video, pipelinedb.StageRun{}, stageOutput{}, err
	}
	row, err := pipelinedb.New(tx).LockStage(ctx, pipelinedb.LockStageParams{VideoID: p.VideoID, Stage: p.Stage, Version: p.Version})
	if errors.Is(err, pgx.ErrNoRows) {
		return video, row, stageOutput{}, ErrStale
	}
	if err != nil {
		return video, row, stageOutput{}, err
	}
	output, err := decodeOutput(row.Output)
	return video, row, output, err
}

func currentAttempt(row pipelinedb.StageRun, attempt int) bool {
	return row.Status == "running" && int(row.Attempt) == attempt
}

func (s *Service) saveProgress(ctx context.Context, p queue.StagePayload, attempt int, v any, commit Commit) error {
	progress, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, s.deps.Pool, func(tx pgx.Tx) error {
		_, row, output, err := lockedStage(ctx, tx, p)
		if err != nil {
			return err
		}
		if !currentAttempt(row, attempt) {
			return ErrStale
		}
		if commit != nil {
			if err := commit(ctx, tx); err != nil {
				return err
			}
		}
		output.Progress = progress
		data, err := json.Marshal(output)
		if err != nil {
			return err
		}
		return pipelinedb.New(tx).UpdateStage(ctx, stageParams(p, "running", data, ""))
	})
}

func (s *Service) complete(ctx context.Context, p queue.StagePayload, attempt int, commit Commit) ([]stageTask, error) {
	var tasks []stageTask
	err := db.WithTx(ctx, s.deps.Pool, func(tx pgx.Tx) error {
		video, row, _, err := lockedStage(ctx, tx, p)
		if err != nil {
			return err
		}
		if !currentAttempt(row, attempt) {
			return ErrStale
		}
		if commit != nil {
			if err := commit(ctx, tx); err != nil {
				return err
			}
		}
		q := pipelinedb.New(tx)
		if err := q.UpdateStage(ctx, stageParams(p, "done", row.Output, "")); err != nil {
			return err
		}
		if p.Stage == "probe" {
			video, err = library.GetForWorker(ctx, tx, p.VideoID)
			if err != nil {
				return err
			}
			plan, err := ExpandPlan(video, s.deps.Channels.List(), s.deps.Registry)
			if err != nil {
				return err
			}
			if err := addPlan(ctx, tx, video, plan); err != nil {
				return err
			}
		}
		tasks, err = advance(ctx, tx, video)
		return err
	})
	return tasks, err
}

func (s *Service) fail(ctx context.Context, p queue.StagePayload, attempt int, cause error) error {
	if errors.Is(cause, mlclient.ErrThrottled) {
		cause = queue.ErrThrottled
	}
	var mlErr *mlclient.Error
	if errors.As(cause, &mlErr) && (mlErr.Status == 503 || mlErr.Status == 504) {
		cause = queue.ErrThrottled
	}
	permanent := errors.Is(cause, asynq.SkipRetry) || (errors.As(cause, &mlErr) && !mlErr.Retryable)
	switch apperr.KindOf(cause) {
	case apperr.Invalid, apperr.NotFound, apperr.Forbidden, apperr.TooLarge:
		permanent = true
	}
	retries, _ := asynq.GetRetryCount(ctx)
	maxRetries, ok := asynq.GetMaxRetry(ctx)
	if !ok {
		maxRetries = s.deps.Worker.MaxRetry
	} else {
		maxRetries-- // The last Asynq slot is reserved for non-failure throttling.
	}
	terminal := !errors.Is(cause, queue.ErrThrottled) && (permanent || retries >= maxRetries)
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	err := db.WithTx(cleanup, s.deps.Pool, func(tx pgx.Tx) error {
		_, row, _, err := lockedStage(cleanup, tx, p)
		if err != nil {
			return err
		}
		if !currentAttempt(row, attempt) {
			return ErrStale
		}
		status := "queued"
		if terminal {
			status = "failed"
		}
		message := "视频处理失败，请稍后重试"
		var public *apperr.Error
		if errors.As(cause, &public) && public.Msg != "" {
			message = public.Msg
		}
		if err := pipelinedb.New(tx).UpdateStage(cleanup, stageParams(p, status, row.Output, message)); err != nil {
			return err
		}
		if terminal && row.Required {
			return library.SetStatus(cleanup, tx, p.VideoID, "failed", message)
		}
		return nil
	})
	if errors.Is(err, ErrStale) {
		return nil
	}
	if err != nil {
		return errors.Join(cause, err)
	}
	if terminal {
		return errors.Join(asynq.SkipRetry, cause)
	}
	return cause
}

var gpuLease = redis.NewScript(`
local clock = redis.call('TIME')
local now = clock[1] * 1000 + math.floor(clock[2] / 1000)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', now)
if redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[1]) then return 0 end
redis.call('ZADD', KEYS[1], now + tonumber(ARGV[2]), ARGV[3])
local last = redis.call('ZREVRANGE', KEYS[1], 0, 0, 'WITHSCORES')
redis.call('PEXPIRE', KEYS[1], math.ceil(tonumber(last[2]) - now))
return 1`)

func (s *Service) acquireGPU(ctx context.Context, workspaceID int64, def StageDef) (func(), error) {
	if def.Queue != queue.QueueGPU {
		return func() {}, nil
	}
	key, token := fmt.Sprintf("vh:gpu:%d", workspaceID), rand.Text()
	acquired, err := gpuLease.Run(ctx, s.deps.Redis, []string{key}, s.deps.Worker.WorkspaceGPULimit, (def.Timeout + time.Minute).Milliseconds(), token).Int()
	if err != nil {
		return nil, err
	}
	if acquired == 0 {
		return nil, queue.ErrThrottled
	}
	return func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := s.deps.Redis.ZRem(cleanup, key, token).Err(); err != nil {
			slog.Warn("release GPU lease", "workspace_id", workspaceID, "error", err)
		}
	}, nil
}
