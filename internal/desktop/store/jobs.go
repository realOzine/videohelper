package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const jobColumns = `id,video_id,status,stage,attempt,progress,error,created_at,updated_at`

func scanJob(row scanner) (Job, error) {
	var j Job
	var progress, created, updated string
	err := row.Scan(&j.ID, &j.VideoID, &j.Status, &j.Stage, &j.Attempt, &progress, &j.Error, &created, &updated)
	if err != nil {
		return j, missing(err)
	}
	j.Progress = json.RawMessage(progress)
	if j.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return j, err
	}
	j.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return j, err
}

func (s *Store) Job(ctx context.Context, id int64) (Job, error) {
	return scanJob(s.db.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM job WHERE id=?", id))
}
func (s *Store) LatestJob(ctx context.Context, videoID int64) (Job, error) {
	return scanJob(s.db.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM job WHERE video_id=? ORDER BY id DESC LIMIT 1", videoID))
}

func (s *Store) ListJobs(ctx context.Context, videoID int64) ([]Job, error) {
	query := "SELECT " + jobColumns + " FROM job"
	args := []any{}
	if videoID > 0 {
		query += " WHERE video_id=?"
		args = append(args, videoID)
	}
	rows, err := s.db.QueryContext(ctx, query+" ORDER BY id DESC", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// CreateJob is idempotent while a video already has a queued or running job.
func (s *Store) CreateJob(ctx context.Context, videoID int64) (Job, error) {
	var j Job
	err := s.write(ctx, func(tx *sql.Tx) error {
		var err error
		j, err = createJob(ctx, tx, videoID)
		return err
	})
	return j, err
}

func createJob(ctx context.Context, tx *sql.Tx, videoID int64) (Job, error) {
	var source, kind string
	if err := tx.QueryRowContext(ctx, "SELECT source_key,source_kind FROM video WHERE id=? AND deleted_at IS NULL", videoID).Scan(&source, &kind); err != nil {
		return Job{}, missing(err)
	}
	existing, err := scanJob(tx.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM job WHERE video_id=? AND status IN ('queued','running')", videoID))
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Job{}, err
	}
	stage, status := "probe", "indexing"
	if source == "" {
		if kind == "upload" {
			return Job{}, ErrConflict
		}
		stage, status = "fetch", "importing"
	}
	now := timestamp()
	result, err := tx.ExecContext(ctx, "INSERT INTO job(video_id,status,stage,created_at,updated_at) VALUES(?,'queued',?,?,?)", videoID, stage, now, now)
	if err != nil {
		return Job{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Job{}, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE video SET status=?,error='',updated_at=? WHERE id=?", status, now, videoID); err != nil {
		return Job{}, err
	}
	return scanJob(tx.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM job WHERE id=?", id))
}

// ClaimJob atomically takes one queued job. ErrNotFound means the queue is idle.
func (s *Store) ClaimJob(ctx context.Context) (Job, error) {
	var j Job
	err := s.write(ctx, func(tx *sql.Tx) error {
		var id int64
		if err := tx.QueryRowContext(ctx, `SELECT j.id FROM job j JOIN video v ON v.id=j.video_id WHERE j.status='queued' AND v.deleted_at IS NULL ORDER BY j.id LIMIT 1`).Scan(&id); err != nil {
			return missing(err)
		}
		if err := changed(tx.ExecContext(ctx, "UPDATE job SET status='running',attempt=attempt+1,updated_at=? WHERE id=? AND status='queued'", timestamp(), id)); err != nil {
			return err
		}
		var err error
		j, err = scanJob(tx.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM job WHERE id=?", id))
		return err
	})
	return j, err
}

// RecoverJobs is called by the single scheduler owner at application startup,
// before launching workers. Incrementing attempt invalidates abandoned writers.
func (s *Store) RecoverJobs(ctx context.Context) (int64, error) {
	var count int64
	err := s.write(ctx, func(tx *sql.Tx) error {
		now := timestamp()
		r, err := tx.ExecContext(ctx, "UPDATE job SET status='queued',attempt=attempt+1,updated_at=? WHERE status='running'", now)
		if err != nil {
			return err
		}
		count, err = r.RowsAffected()
		if err != nil {
			return err
		}
		// Repair the import/enqueue crash gap; any prior job, including a user's
		// cancellation, prevents automatic recreation of that video job.
		r, err = tx.ExecContext(ctx, `INSERT INTO job(video_id,status,stage,created_at,updated_at)
 SELECT v.id,'queued',CASE WHEN v.source_key='' THEN 'fetch' ELSE 'probe' END,?,?
 FROM video v WHERE v.deleted_at IS NULL AND v.status IN ('importing','indexing')
 AND (v.source_kind<>'upload' OR v.source_key<>'')
 AND NOT EXISTS(SELECT 1 FROM job j WHERE j.video_id=v.id)`, now, now)
		if err != nil {
			return err
		}
		n, err := r.RowsAffected()
		count += n
		return err
	})
	return count, err
}

func currentJob(ctx context.Context, tx *sql.Tx, id int64, attempt int) (Job, error) {
	j, err := scanJob(tx.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM job WHERE id=?", id))
	if err != nil {
		return j, err
	}
	if j.Status != "running" || j.Attempt != attempt {
		return j, ErrStale
	}
	var deleted sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT deleted_at FROM video WHERE id=?", j.VideoID).Scan(&deleted); err != nil {
		return j, missing(err)
	}
	if deleted.Valid {
		return j, ErrStale
	}
	return j, nil
}

// Checkpoint commits derived rows and progress together, after fencing out old
// or cancelled attempts. Do file/model I/O before entering this transaction.
func (s *Store) Checkpoint(ctx context.Context, id int64, attempt int, stage string, progress any, output func(*Tx) error) error {
	if strings.TrimSpace(stage) == "" {
		return ErrInvalid
	}
	data, err := json.Marshal(progress)
	if err != nil {
		return err
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		if _, err := currentJob(ctx, tx, id, attempt); err != nil {
			return err
		}
		if output != nil {
			if err := output(&Tx{tx}); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, "UPDATE job SET stage=?,progress=?,updated_at=? WHERE id=?", stage, string(data), timestamp(), id)
		return err
	})
}

func (s *Store) CompleteJob(ctx context.Context, id int64, attempt int, output func(*Tx) error) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		j, err := currentJob(ctx, tx, id, attempt)
		if err != nil {
			return err
		}
		if output != nil {
			if err := output(&Tx{tx}); err != nil {
				return err
			}
		}
		now := timestamp()
		if _, err := tx.ExecContext(ctx, "UPDATE video SET status='ready',error='',updated_at=? WHERE id=?", now, j.VideoID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE job SET status='done',stage='finalize',error='',updated_at=? WHERE id=?", now, id)
		return err
	})
}

func (s *Store) FailJob(ctx context.Context, id int64, attempt int, message string) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		j, err := currentJob(ctx, tx, id, attempt)
		if err != nil {
			return err
		}
		now := timestamp()
		if _, err := tx.ExecContext(ctx, "UPDATE job SET status='failed',error=?,updated_at=? WHERE id=?", message, now, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE video SET status='failed',error=?,updated_at=? WHERE id=?", message, now, j.VideoID)
		return err
	})
}

func (s *Store) CancelJob(ctx context.Context, id int64) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		j, err := scanJob(tx.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM job WHERE id=?", id))
		if err != nil {
			return err
		}
		if j.Status != "queued" && j.Status != "running" {
			return ErrConflict
		}
		now := timestamp()
		if _, err := tx.ExecContext(ctx, "UPDATE job SET status='cancelled',updated_at=? WHERE id=?", now, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE video SET status='failed',error='任务已取消',updated_at=? WHERE id=? AND deleted_at IS NULL", now, j.VideoID)
		return err
	})
}

func (s *Store) RetryJob(ctx context.Context, id int64) (Job, error) {
	var j Job
	err := s.write(ctx, func(tx *sql.Tx) error {
		old, err := scanJob(tx.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM job WHERE id=?", id))
		if err != nil {
			return err
		}
		if old.Status != "failed" && old.Status != "cancelled" {
			return ErrConflict
		}
		var source string
		if err := tx.QueryRowContext(ctx, "SELECT source_key FROM video WHERE id=? AND deleted_at IS NULL", old.VideoID).Scan(&source); err != nil {
			return missing(err)
		}
		var active int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM job WHERE video_id=? AND status IN ('queued','running')", old.VideoID).Scan(&active); err != nil {
			return err
		}
		if active != 0 {
			return ErrConflict
		}
		now := timestamp()
		status := "indexing"
		if source == "" {
			status = "importing"
		}
		if _, err := tx.ExecContext(ctx, "UPDATE job SET status='queued',error='',updated_at=? WHERE id=?", now, id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "UPDATE video SET status=?,error='',updated_at=? WHERE id=?", status, now, old.VideoID); err != nil {
			return err
		}
		j, err = scanJob(tx.QueryRowContext(ctx, "SELECT "+jobColumns+" FROM job WHERE id=?", id))
		return err
	})
	return j, err
}
