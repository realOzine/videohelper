package pipeline

import (
	"context"
	"errors"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ozine/videohelper/internal/pipeline/pipelinedb"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/platform/queue"
)

// Reconcile repairs deliveries and uploads, and fills missing embedding stages on ready videos.
func (s *Service) Reconcile(ctx context.Context) error {
	now := time.Now()
	q := pipelinedb.New(s.deps.Pool)
	rows, err := q.ReconcileStages(ctx, pipelinedb.ReconcileStagesParams{
		BeforeTime: pgtype.Timestamptz{Time: now.Add(-10 * time.Minute), Valid: true},
		NowTime:    pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return err
	}
	var errs []error
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if err := s.recoverStage(ctx, row); err != nil {
			errs = append(errs, err)
		}
	}
	uploads, err := q.UnstartedUploads(ctx)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	for _, id := range uploads {
		if err := s.Start(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}
	if err := s.backfill(ctx); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (s *Service) recoverStage(ctx context.Context, snapshot pipelinedb.ReconcileStagesRow) error {
	output, err := decodeOutput(snapshot.Output)
	if err != nil {
		return err
	}
	p := queue.StagePayload{WorkspaceID: snapshot.WorkspaceID, VideoID: snapshot.VideoID, Stage: snapshot.Stage, Version: snapshot.Version}
	old := stageTask{payload: p, def: output.Plan}
	info, err := s.taskInfo(old)
	if err != nil {
		return err
	}
	if info != nil {
		if info.State == asynq.TaskStateActive {
			return nil
		}
		if snapshot.Status == "queued" && info.State != asynq.TaskStateArchived && info.State != asynq.TaskStateCompleted {
			return nil
		}
	}
	retire := snapshot.Status == "running" || info != nil
	var next stageTask
	err = db.WithTx(ctx, s.deps.Pool, func(tx pgx.Tx) error {
		_, row, current, err := lockedStage(ctx, tx, p)
		if err != nil {
			return err
		}
		if row.Status != snapshot.Status || row.Attempt != snapshot.Attempt || !row.UpdatedAt.Time.Equal(snapshot.UpdatedAt.Time) {
			return ErrStale
		}
		q := pipelinedb.New(tx)
		if retire {
			p.Version = generation(current.Plan.Version)
			if err := q.RetryStage(ctx, pipelinedb.RetryStageParams{VideoID: row.VideoID, Stage: row.Stage, Version: row.Version, NewVersion: p.Version, NewStatus: "queued"}); err != nil {
				return err
			}
		} else if err := q.UpdateStage(ctx, stageParams(p, "queued", row.Output, "")); err != nil {
			return err
		}
		next = stageTask{payload: p, def: current.Plan}
		return nil
	})
	if errors.Is(err, ErrStale) {
		return nil
	}
	if err != nil {
		return err
	}
	var cleanup error
	if retire {
		cleanup = s.removeTask(old)
	}
	return errors.Join(cleanup, s.enqueue(ctx, []stageTask{next}))
}
