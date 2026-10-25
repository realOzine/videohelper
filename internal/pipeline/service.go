package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"maps"
	"strings"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/pipeline/pipelinedb"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/platform/queue"
)

func New(deps Deps) *Service {
	deps.Runs = maps.Clone(deps.Runs)
	return &Service{deps: deps}
}

type stageTask struct {
	payload queue.StagePayload
	def     StageDef
}

func generation(version string) string { return version + "." + strings.ToLower(rand.Text()) }

func (s *Service) Start(ctx context.Context, videoID int64) error {
	var tasks []stageTask
	err := db.WithTx(ctx, s.deps.Pool, func(tx pgx.Tx) error {
		video, err := library.GetForWorker(ctx, tx, videoID)
		if err != nil {
			return err
		}
		if video.SourceKind != "upload" || video.SourceKey == "" {
			return apperr.New(apperr.Conflict, "not_ready", "原片尚未上传完成")
		}
		rows, err := pipelinedb.New(tx).Stages(ctx, videoID)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			if video.Status != "uploading" && video.Status != "indexing" {
				return apperr.New(apperr.Conflict, "conflict", "当前状态不能启动索引")
			}
			if err := library.SetStatus(ctx, tx, videoID, "indexing", ""); err != nil {
				return err
			}
			if err := addPlan(ctx, tx, video, InitialPlan(video)); err != nil {
				return err
			}
		}
		tasks, err = advance(ctx, tx, video)
		return err
	})
	if err != nil {
		return err
	}
	return s.enqueue(ctx, tasks)
}

func (s *Service) DropDerived(ctx context.Context, q db.Querier, videoID int64) error {
	if err := index.DeleteVideo(ctx, q, videoID); err != nil {
		return err
	}
	return pipelinedb.New(q).DeleteStages(ctx, videoID)
}

func (s *Service) Retry(ctx context.Context, workspaceID, videoID int64) error {
	var tasks, old []stageTask
	err := db.WithTx(ctx, s.deps.Pool, func(tx pgx.Tx) error {
		video, err := library.GetForWorker(ctx, tx, videoID)
		if err != nil {
			return err
		}
		if video.WorkspaceID != workspaceID {
			return apperr.New(apperr.NotFound, "not_found", "视频不存在")
		}
		if video.Status != "failed" {
			return apperr.New(apperr.Conflict, "conflict", "只能重试失败的视频")
		}
		q := pipelinedb.New(tx)
		rows, err := q.Stages(ctx, videoID)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.Status == "done" {
				continue
			}
			output, err := decodeOutput(row.Output)
			if err != nil {
				return err
			}
			old = append(old, taskFor(video, row, output.Plan))
			status := "pending"
			if dependenciesDone(output.Plan.Deps, rows) {
				status = "queued"
			}
			if err := q.RetryStage(ctx, pipelinedb.RetryStageParams{VideoID: videoID, Stage: row.Stage, Version: row.Version,
				NewVersion: generation(output.Plan.Version), NewStatus: status}); err != nil {
				return err
			}
		}
		if len(old) == 0 {
			return apperr.New(apperr.Conflict, "conflict", "没有可以重试的阶段")
		}
		if err := library.SetStatus(ctx, tx, videoID, "indexing", ""); err != nil {
			return err
		}
		tasks, err = advance(ctx, tx, video)
		return err
	})
	if err != nil {
		return err
	}
	var cleanup []error
	for _, task := range old {
		if err := ctx.Err(); err != nil {
			cleanup = append(cleanup, err)
			break
		}
		if err := s.removeTask(task); err != nil {
			cleanup = append(cleanup, err)
		}
	}
	return errors.Join(append(cleanup, s.enqueue(ctx, tasks))...)
}

func (s *Service) taskInfo(task stageTask) (*asynq.TaskInfo, error) {
	info, err := s.deps.Inspector.GetTaskInfo(task.def.Queue, queue.StageTaskID(task.payload))
	if errors.Is(err, asynq.ErrTaskNotFound) || errors.Is(err, asynq.ErrQueueNotFound) {
		return nil, nil
	}
	return info, err
}

func (s *Service) removeTask(task stageTask) error {
	info, err := s.taskInfo(task)
	if err != nil || info == nil || info.State == asynq.TaskStateActive {
		return err
	}
	err = s.deps.Inspector.DeleteTask(task.def.Queue, queue.StageTaskID(task.payload))
	if errors.Is(err, asynq.ErrTaskNotFound) || errors.Is(err, asynq.ErrQueueNotFound) {
		return nil
	}
	return err
}

func addPlan(ctx context.Context, tx pgx.Tx, video library.Video, plan []StageDef) error {
	q := pipelinedb.New(tx)
	rows, err := q.Stages(ctx, video.ID)
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		seen[row.Stage] = true
	}
	for _, def := range plan {
		if seen[def.Name] {
			continue
		}
		output, err := json.Marshal(stageOutput{Plan: def})
		if err != nil {
			return err
		}
		if err := q.InsertStage(ctx, pipelinedb.InsertStageParams{VideoID: video.ID, Stage: def.Name, Version: generation(def.Version),
			Status: "pending", Required: def.Required, Output: output}); err != nil {
			return err
		}
		seen[def.Name] = true
	}
	return nil
}

func advance(ctx context.Context, tx pgx.Tx, video library.Video) ([]stageTask, error) {
	q := pipelinedb.New(tx)
	rows, err := q.Stages(ctx, video.ID)
	if err != nil {
		return nil, err
	}
	var tasks []stageTask
	for _, row := range rows {
		output, err := decodeOutput(row.Output)
		if err != nil {
			return nil, err
		}
		if row.Status == "pending" && dependenciesDone(output.Plan.Deps, rows) {
			if err := q.UpdateStage(ctx, pipelinedb.UpdateStageParams{VideoID: row.VideoID, Stage: row.Stage, Version: row.Version, Status: "queued", Output: row.Output}); err != nil {
				return nil, err
			}
			row.Status = "queued"
		}
		if row.Status == "queued" {
			tasks = append(tasks, taskFor(video, row, output.Plan))
		}
	}
	return tasks, nil
}

func dependenciesDone(deps []string, rows []pipelinedb.StageRun) bool {
	for _, dep := range deps {
		found := false
		for _, row := range rows {
			if row.Stage == dep && row.Status == "done" {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func taskFor(v library.Video, row pipelinedb.StageRun, def StageDef) stageTask {
	return stageTask{payload: queue.StagePayload{WorkspaceID: v.WorkspaceID, VideoID: v.ID, Stage: row.Stage, Version: row.Version}, def: def}
}

func decodeOutput(data []byte) (stageOutput, error) {
	var output stageOutput
	err := json.Unmarshal(data, &output)
	return output, err
}

func (s *Service) enqueue(ctx context.Context, tasks []stageTask) error {
	var errs []error
	for _, task := range tasks {
		// Asynq checks exhaustion before IsFailure; reserve one attempt so throttling can always retry.
		if err := queue.EnqueueStage(ctx, s.deps.Client, task.payload, task.def.Queue, task.def.Timeout, s.deps.Worker.MaxRetry+1); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func stageParams(p queue.StagePayload, status string, output []byte, errMsg string) pipelinedb.UpdateStageParams {
	return pipelinedb.UpdateStageParams{VideoID: p.VideoID, Stage: p.Stage, Version: p.Version, Status: status, Output: output,
		Error: pgtype.Text{String: errMsg, Valid: errMsg != ""}}
}
