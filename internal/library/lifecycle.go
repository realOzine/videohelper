package library

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/library/librarydb"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/workspace"
)

func (s *Service) Delete(ctx context.Context, p workspace.Principal, id int64) error {
	return db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := librarydb.New(tx)
		video, err := q.VideoIncludingDeleted(ctx, librarydb.VideoIncludingDeletedParams{ID: id, WorkspaceID: p.WorkspaceID})
		if err != nil {
			return rowError(err)
		}
		if video.DeletedAt.Valid {
			return nil
		}
		if s.dropper == nil {
			return unavailable()
		}
		if err := q.SoftDelete(ctx, librarydb.SoftDeleteParams{ID: id, WorkspaceID: p.WorkspaceID}); err != nil {
			return err
		}
		return s.dropper.DropDerived(ctx, tx, id)
	})
}

func (s *Service) Restore(ctx context.Context, p workspace.Principal, id int64) (Video, error) {
	var sourceKnown bool
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := librarydb.New(tx)
		row, err := q.VideoIncludingDeleted(ctx, librarydb.VideoIncludingDeletedParams{ID: id, WorkspaceID: p.WorkspaceID})
		if err != nil {
			return rowError(err)
		}
		if !row.DeletedAt.Valid || row.SourceKind != "upload" {
			return apperr.New(apperr.Conflict, "invalid_restore_state", "当前状态不能恢复")
		}
		sourceKnown = row.SourceKey.Valid && row.SourceKey.String != ""
		// uploading + SourceKey lets the reconciler repair a failed Start after restoration.
		return q.RestoreVideo(ctx, librarydb.RestoreVideoParams{ID: id, WorkspaceID: p.WorkspaceID, Status: "uploading"})
	})
	if err != nil {
		return Video{}, err
	}
	if sourceKnown {
		if s.starter == nil {
			return Video{}, unavailable()
		}
		if err := s.starter.Start(ctx, id); err != nil {
			return Video{}, err
		}
	}
	return s.Get(ctx, p, id)
}

func (s *Service) PlaybackURL(ctx context.Context, p workspace.Principal, id int64) (string, time.Time, error) {
	video, err := s.Get(ctx, p, id)
	if err != nil {
		return "", time.Time{}, err
	}
	if video.NormalizedVideoKey == "" {
		return "", time.Time{}, apperr.New(apperr.Conflict, "media_not_ready", "视频尚未准备完成")
	}
	if s.objects == nil {
		return "", time.Time{}, unavailable()
	}
	expiresAt := time.Now().Add(s.cfg.PresignTTL)
	url, err := s.objects.PresignGet(ctx, video.NormalizedVideoKey, s.cfg.PresignTTL)
	if err != nil {
		return "", time.Time{}, err
	}
	return url, expiresAt, nil
}
