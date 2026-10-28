package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Keep the v1 on-disk status constraint. A cancelled latest job distinguishes
// user cancellation from failure without rebuilding existing SQLite libraries.
// Only failed videos derive this state, so older cancelled jobs cannot mask a
// later retry, successful index, or a genuine failure.
const videoStatus = `CASE WHEN video.status='failed' AND
 (SELECT j.status FROM job j WHERE j.video_id=video.id ORDER BY j.id DESC LIMIT 1)='cancelled'
 THEN 'cancelled' ELSE video.status END`

const videoColumns = `id,title,source_kind,source_url,source_filename,source_key,normalized_video_key,duration_ms,has_video,has_audio,thumbs,` + videoStatus + `,error,deleted_at,created_at,updated_at`

type scanner interface{ Scan(...any) error }

func scanVideo(row scanner) (Video, error) {
	var v Video
	var duration sql.NullInt64
	var hasVideo, hasAudio sql.NullBool
	var thumbs, deleted sql.NullString
	var created, updated string
	err := row.Scan(&v.ID, &v.Title, &v.SourceKind, &v.SourceURL, &v.SourceFilename, &v.SourceKey, &v.NormalizedVideoKey, &duration, &hasVideo, &hasAudio, &thumbs, &v.Status, &v.Error, &deleted, &created, &updated)
	if err != nil {
		return v, missing(err)
	}
	if duration.Valid {
		n := int(duration.Int64)
		v.DurationMs = &n
	}
	if hasVideo.Valid {
		v.HasVideo = &hasVideo.Bool
	}
	if hasAudio.Valid {
		v.HasAudio = &hasAudio.Bool
	}
	if thumbs.Valid {
		if err := json.Unmarshal([]byte(thumbs.String), &v.Thumbs); err != nil {
			return v, err
		}
	}
	if deleted.Valid {
		t, err := time.Parse(time.RFC3339Nano, deleted.String)
		if err != nil {
			return v, err
		}
		v.DeletedAt = &t
	}
	if v.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return v, err
	}
	v.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	return v, err
}

func (s *Store) Video(ctx context.Context, id int64) (Video, error) {
	return scanVideo(s.db.QueryRowContext(ctx, "SELECT "+videoColumns+" FROM video WHERE id=?", id))
}

func placeholders(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

func (s *Store) ListVideos(ctx context.Context, f VideoFilter) ([]Video, error) {
	if f.Limit < 0 {
		return nil, ErrInvalid
	}
	if f.IDs != nil && len(f.IDs) == 0 {
		return []Video{}, nil
	}
	query := "SELECT " + videoColumns + " FROM video WHERE deleted_at IS NULL"
	if f.Deleted {
		query = "SELECT " + videoColumns + " FROM video WHERE deleted_at IS NOT NULL"
	}
	args := []any{}
	if f.BeforeID != nil {
		query += " AND id<?"
		args = append(args, *f.BeforeID)
	}
	if f.Status != "" {
		query += " AND (" + videoStatus + ")=?"
		args = append(args, f.Status)
	}
	if f.ProjectID != nil {
		query += " AND EXISTS (SELECT 1 FROM project_video p WHERE p.video_id=video.id AND p.project_id=?)"
		args = append(args, *f.ProjectID)
	}
	if f.IDs != nil {
		query += " AND id IN (" + placeholders(len(f.IDs)) + ")"
		for _, id := range f.IDs {
			args = append(args, id)
		}
	}
	query += " ORDER BY id DESC"
	if f.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, f.Limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	videos := []Video{}
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		videos = append(videos, v)
	}
	return videos, rows.Err()
}

func insertVideo(ctx context.Context, tx *sql.Tx, kind, sourceURL, title, filename string, projectID *int64) (int64, error) {
	if strings.TrimSpace(title) == "" {
		return 0, ErrInvalid
	}
	now, status := timestamp(), "importing"
	if kind == "upload" {
		status = "uploading"
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO video(title,source_kind,source_url,source_filename,status,created_at,updated_at) VALUES(?,?,?,?,?,?,?)`, title, kind, sourceURL, filename, status, now, now)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if projectID != nil {
		_, err = tx.ExecContext(ctx, "INSERT INTO project_video(project_id,video_id) VALUES(?,?)", *projectID, id)
	}
	return id, err
}

func (s *Store) CreateImport(ctx context.Context, sourceURL, kind, title string, projectID *int64) (Video, error) {
	u, err := url.Parse(sourceURL)
	if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || (kind != "youtube" && kind != "bilibili" && kind != "direct") {
		return Video{}, ErrInvalid
	}
	if strings.TrimSpace(title) == "" {
		title = sourceURL
	}
	var v Video
	err = s.write(ctx, func(tx *sql.Tx) error {
		id, err := insertVideo(ctx, tx, kind, sourceURL, title, "", projectID)
		if err != nil {
			return err
		}
		if _, err := createJob(ctx, tx, id); err != nil {
			return err
		}
		v, err = scanVideo(tx.QueryRowContext(ctx, "SELECT "+videoColumns+" FROM video WHERE id=?", id))
		return err
	})
	return v, err
}

func (s *Store) ImportURL(ctx context.Context, raw string, projectID *int64) (Video, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return Video{}, ErrInvalid
	}
	host, kind := strings.ToLower(u.Hostname()), "direct"
	if host == "youtube.com" || strings.HasSuffix(host, ".youtube.com") || host == "youtu.be" {
		kind = "youtube"
	}
	if host == "bilibili.com" || strings.HasSuffix(host, ".bilibili.com") || host == "b23.tv" {
		kind = "bilibili"
	}
	return s.CreateImport(ctx, raw, kind, raw, projectID)
}

func (s *Store) ImportFile(ctx context.Context, path string, projectID *int64) (Video, error) {
	name := filepath.Base(path)
	var id int64
	err := s.write(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = insertVideo(ctx, tx, "upload", "", name, name, projectID)
		return err
	})
	if err != nil {
		return Video{}, err
	}
	key, err := s.CopyFile(ctx, id, path)
	var v Video
	if err == nil {
		v, err = s.commitFileImport(ctx, id, key)
	}
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return Video{}, fmt.Errorf("copy import: %w", errors.Join(err, s.discardImport(cleanup, id)))
	}
	return v, nil
}

func (s *Store) commitFileImport(ctx context.Context, id int64, key string) (Video, error) {
	// A complete managed copy must reach the durable queue even if its caller disconnects.
	commit, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var v Video
	err := s.write(commit, func(tx *sql.Tx) error {
		if err := (&Tx{tx}).SetSource(commit, id, key); err != nil {
			return err
		}
		if _, err := createJob(commit, tx, id); err != nil {
			return err
		}
		var err error
		v, err = scanVideo(tx.QueryRowContext(commit, "SELECT "+videoColumns+" FROM video WHERE id=?", id))
		return err
	})
	return v, err
}

func (s *Store) discardImport(ctx context.Context, id int64) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		var unstarted bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM video v WHERE v.id=? AND v.source_kind='upload' AND v.source_key='' AND NOT EXISTS(SELECT 1 FROM job j WHERE j.video_id=v.id))`, id).Scan(&unstarted); err != nil {
			return err
		}
		if !unstarted {
			return nil
		}
		if err := s.root.RemoveAll("videos/" + strconv.FormatInt(id, 10)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM video WHERE id=?", id)
		return err
	})
}

func validVideoKey(id int64, key string) bool {
	return id > 0 && validKey(key) == nil && strings.HasPrefix(key, "videos/"+strconv.FormatInt(id, 10)+"/")
}

func (t *Tx) SetSource(ctx context.Context, id int64, key string) error {
	if !validVideoKey(id, key) {
		return ErrInvalid
	}
	return changed(t.tx.ExecContext(ctx, "UPDATE video SET source_key=?,status='indexing',updated_at=? WHERE id=? AND deleted_at IS NULL", key, timestamp(), id))
}

func (t *Tx) UpdateMedia(ctx context.Context, id int64, info MediaInfo) error {
	if info.DurationMs <= 0 || info.DurationMs > math.MaxInt32 || (!info.HasAudio && !info.HasVideo) || (info.NormalizedVideoKey != "" && !validVideoKey(id, info.NormalizedVideoKey)) {
		return ErrInvalid
	}
	var thumbs any
	if info.Thumbs != nil {
		b, err := json.Marshal(info.Thumbs)
		if err != nil {
			return err
		}
		thumbs = string(b)
	}
	return changed(t.tx.ExecContext(ctx, `UPDATE video SET duration_ms=?,has_video=?,has_audio=?,normalized_video_key=CASE WHEN ?='' THEN normalized_video_key ELSE ? END,thumbs=coalesce(?,thumbs),updated_at=? WHERE id=? AND deleted_at IS NULL`, info.DurationMs, info.HasVideo, info.HasAudio, info.NormalizedVideoKey, info.NormalizedVideoKey, thumbs, timestamp(), id))
}

func (s *Store) UpdateMedia(ctx context.Context, id int64, info MediaInfo) error {
	return s.write(ctx, func(tx *sql.Tx) error { return (&Tx{tx}).UpdateMedia(ctx, id, info) })
}

func (s *Store) DeleteVideo(ctx context.Context, id int64) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		if err := changed(tx.ExecContext(ctx, `UPDATE video SET deleted_at=coalesce(deleted_at,?),
 status=CASE WHEN status IN ('uploading','importing','indexing') THEN 'failed' ELSE status END,
 error=CASE WHEN status IN ('uploading','importing','indexing') THEN '任务已取消' ELSE error END,
 updated_at=? WHERE id=?`, timestamp(), timestamp(), id)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "UPDATE job SET status='cancelled',updated_at=? WHERE video_id=? AND status IN ('queued','running')", timestamp(), id)
		return err
	})
}

func (s *Store) RestoreVideo(ctx context.Context, id int64) (Video, error) {
	err := changed(s.db.ExecContext(ctx, "UPDATE video SET deleted_at=NULL,updated_at=? WHERE id=? AND deleted_at IS NOT NULL", timestamp(), id))
	if err != nil {
		return Video{}, err
	}
	return s.Video(ctx, id)
}

// PurgeVideo removes only generated paths under this store's media root. Files
// go first, so a failed delete leaves the database tombstone available to retry.
func (s *Store) PurgeVideo(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrInvalid
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		var deleted sql.NullString
		if err := tx.QueryRowContext(ctx, "SELECT deleted_at FROM video WHERE id=?", id).Scan(&deleted); err != nil {
			return missing(err)
		}
		if !deleted.Valid {
			return ErrConflict
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.root.RemoveAll("videos/" + strconv.FormatInt(id, 10)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM video WHERE id=? AND deleted_at IS NOT NULL", id)
		return err
	})
}
