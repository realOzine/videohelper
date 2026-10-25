package library

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/library/librarydb"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/timeline"
)

type Video struct {
	ID                 int64                  `json:"id,string"`
	WorkspaceID        int64                  `json:"workspace_id,string"`
	Title              string                 `json:"title"`
	Status             string                 `json:"status"`
	SourceKind         string                 `json:"source_kind"`
	SourceURL          string                 `json:"source_url,omitempty"`
	SourceKey          string                 `json:"-"`
	NormalizedVideoKey string                 `json:"-"`
	Error              string                 `json:"error,omitempty"`
	DurationMs         *int                   `json:"duration_ms"`
	HasVideo           *bool                  `json:"has_video"`
	HasAudio           *bool                  `json:"has_audio"`
	Thumbs             *timeline.SpriteLayout `json:"thumbs"`
	DeletedAt          *time.Time             `json:"deleted_at,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

type Project struct {
	ID          int64     `json:"id,string"`
	WorkspaceID int64     `json:"workspace_id,string"`
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"created_at"`
}

type Config struct {
	UploadPartBytes int64
	PresignTTL      time.Duration
}

type Service struct {
	pool    *pgxpool.Pool
	objects *blob.Store
	cfg     Config
	starter Starter
	dropper DerivedDropper
}

func New(pool *pgxpool.Pool, objects *blob.Store, cfg Config, starter Starter, dropper DerivedDropper) *Service {
	if cfg.UploadPartBytes == 0 {
		cfg.UploadPartBytes = 16 << 20
	}
	if cfg.PresignTTL == 0 {
		cfg.PresignTTL = time.Hour
	}
	return &Service{pool: pool, objects: objects, cfg: cfg, starter: starter, dropper: dropper}
}

type Starter interface {
	Start(ctx context.Context, videoID int64) error
}

type DerivedDropper interface {
	DropDerived(ctx context.Context, q db.Querier, videoID int64) error
}

func notFound() error { return apperr.New(apperr.NotFound, "not_found", "资源不存在") }

func rowError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound()
	}
	return err
}

func affected(rows int64, err error) error {
	if err != nil {
		return err
	}
	if rows == 0 {
		return notFound()
	}
	return nil
}

func videoFromRow(row librarydb.Video) (Video, error) {
	v := Video{ID: row.ID, WorkspaceID: row.WorkspaceID, Title: row.Title, Status: row.Status,
		SourceKind: row.SourceKind, SourceURL: row.SourceUrl.String, SourceKey: row.SourceKey.String,
		NormalizedVideoKey: row.NormalizedVideoKey.String, Error: row.Error.String,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time}
	if row.DurationMs.Valid {
		value := int(row.DurationMs.Int32)
		v.DurationMs = &value
	}
	if row.HasVideo.Valid {
		v.HasVideo = &row.HasVideo.Bool
	}
	if row.HasAudio.Valid {
		v.HasAudio = &row.HasAudio.Bool
	}
	if row.DeletedAt.Valid {
		v.DeletedAt = &row.DeletedAt.Time
	}
	if len(row.Thumbs) > 0 {
		var layout timeline.SpriteLayout
		if err := json.Unmarshal(row.Thumbs, &layout); err != nil {
			return Video{}, err
		}
		v.Thumbs = &layout
	}
	return v, nil
}

func projectFromRow(row librarydb.Project) Project {
	return Project{ID: row.ID, WorkspaceID: row.WorkspaceID, Name: row.Name, CreatedAt: row.CreatedAt.Time}
}
