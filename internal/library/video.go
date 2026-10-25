package library

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/library/librarydb"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/timeline"
	"github.com/ozine/videohelper/internal/workspace"
)

type ListFilter struct {
	ProjectID *int64
	Status    string
	Limit     int
	Cursor    string
}

func (s *Service) Get(ctx context.Context, p workspace.Principal, id int64) (Video, error) {
	row, err := librarydb.New(s.pool).Video(ctx, librarydb.VideoParams{ID: id, WorkspaceID: p.WorkspaceID})
	if err != nil {
		return Video{}, rowError(err)
	}
	return videoFromRow(row)
}

type cursor struct {
	Time time.Time `json:"t"`
	ID   int64     `json:"id"`
}

func (s *Service) List(ctx context.Context, p workspace.Principal, f ListFilter) ([]Video, string, error) {
	if f.Limit == 0 {
		f.Limit = 20
	}
	if f.Limit < 1 || f.Limit > 100 || (f.Status != "" && !validStatus(f.Status)) {
		return nil, "", apperr.New(apperr.Invalid, "invalid_filter", "列表筛选条件不合法")
	}
	params := librarydb.ListVideosParams{WorkspaceID: p.WorkspaceID, Status: f.Status, PageLimit: int32(f.Limit + 1)}
	if f.ProjectID != nil {
		if _, err := librarydb.New(s.pool).Project(ctx, librarydb.ProjectParams{ID: *f.ProjectID, WorkspaceID: p.WorkspaceID}); err != nil {
			return nil, "", rowError(err)
		}
		params.ProjectID = pgtype.Int8{Int64: *f.ProjectID, Valid: true}
	}
	if f.Cursor != "" {
		var before cursor
		if len(f.Cursor) > 256 {
			return nil, "", apperr.New(apperr.Invalid, "invalid_cursor", "分页游标不合法")
		}
		data, err := base64.RawURLEncoding.DecodeString(f.Cursor)
		if err != nil || json.Unmarshal(data, &before) != nil || before.ID <= 0 || before.Time.IsZero() {
			return nil, "", apperr.New(apperr.Invalid, "invalid_cursor", "分页游标不合法")
		}
		params.BeforeID = before.ID
		params.BeforeTime = pgtype.Timestamptz{Time: before.Time, Valid: true}
	}
	rows, err := librarydb.New(s.pool).ListVideos(ctx, params)
	if err != nil {
		return nil, "", err
	}
	var next string
	if len(rows) > f.Limit {
		rows = rows[:f.Limit]
		last := rows[len(rows)-1]
		data, err := json.Marshal(cursor{Time: last.CreatedAt.Time, ID: last.ID})
		if err != nil {
			return nil, "", err
		}
		next = base64.RawURLEncoding.EncodeToString(data)
	}
	videos := make([]Video, 0, len(rows))
	for _, row := range rows {
		video, err := videoFromRow(row)
		if err != nil {
			return nil, "", err
		}
		videos = append(videos, video)
	}
	return videos, next, nil
}

// GetForWorker locks the active video row; pass the same transaction used for derived writes.
func GetForWorker(ctx context.Context, q db.Querier, videoID int64) (Video, error) {
	row, err := librarydb.New(q).VideoForWorker(ctx, videoID)
	if err != nil {
		return Video{}, rowError(err)
	}
	return videoFromRow(row)
}

func UpdateSource(ctx context.Context, q db.Querier, videoID int64, sourceKey string) error {
	if sourceKey == "" {
		return apperr.New(apperr.Invalid, "invalid_source", "原片路径不能为空")
	}
	return affected(librarydb.New(q).UpdateSource(ctx, librarydb.UpdateSourceParams{ID: videoID, SourceKey: pgtype.Text{String: sourceKey, Valid: true}}))
}

func UpdateProbe(ctx context.Context, q db.Querier, videoID int64, durationMs int, hasVideo, hasAudio bool) error {
	if durationMs <= 0 || durationMs > math.MaxInt32 || (!hasVideo && !hasAudio) {
		return apperr.New(apperr.Invalid, "invalid_media", "无法识别的视频文件")
	}
	return affected(librarydb.New(q).UpdateProbe(ctx, librarydb.UpdateProbeParams{ID: videoID,
		DurationMs: pgtype.Int4{Int32: int32(durationMs), Valid: true}, HasVideo: pgtype.Bool{Bool: hasVideo, Valid: true}, HasAudio: pgtype.Bool{Bool: hasAudio, Valid: true}}))
}

func UpdatePreparedMedia(ctx context.Context, q db.Querier, videoID int64, normalizedVideoKey string, thumbs timeline.SpriteLayout) error {
	if normalizedVideoKey == "" || thumbs.TileW <= 0 || thumbs.TileH <= 0 || thumbs.Cols <= 0 || thumbs.Rows <= 0 || thumbs.Count < 0 {
		return apperr.New(apperr.Invalid, "invalid_prepared_media", "标准化媒体信息不合法")
	}
	data, err := json.Marshal(thumbs)
	if err != nil {
		return err
	}
	return affected(librarydb.New(q).UpdatePreparedMedia(ctx, librarydb.UpdatePreparedMediaParams{ID: videoID,
		NormalizedVideoKey: pgtype.Text{String: normalizedVideoKey, Valid: true}, Thumbs: data}))
}

func validStatus(status string) bool {
	return slices.Contains([]string{"uploading", "importing", "indexing", "ready", "failed"}, status)
}

func SetStatus(ctx context.Context, q db.Querier, videoID int64, status, errMsg string) error {
	if !validStatus(status) {
		return apperr.New(apperr.Invalid, "invalid_status", "视频状态不合法")
	}
	return affected(librarydb.New(q).SetStatus(ctx, librarydb.SetStatusParams{ID: videoID, Status: status, Error: pgtype.Text{String: errMsg, Valid: errMsg != ""}}))
}
