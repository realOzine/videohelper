package library

import (
	"cmp"
	"context"
	"errors"
	"mime"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/library/librarydb"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/workspace"
)

const (
	mebibyte       = int64(1 << 20)
	maxObjectBytes = int64(5 << 40)
)

type UploadPlan struct {
	UploadID  string         `json:"upload_id"`
	PartSize  int64          `json:"part_size"`
	Parts     []blob.PartURL `json:"parts"`
	ExpiresAt time.Time      `json:"expires_at"`
}

func unavailable() error {
	return apperr.New(apperr.Unavailable, "service_unavailable", "服务暂时不可用")
}

func uploadSizing(size, configuredPart int64) (partSize int64, count int, err error) {
	if size <= 0 || configuredPart < 5*mebibyte || configuredPart > 5<<30 {
		return 0, 0, apperr.New(apperr.Invalid, "invalid_upload", "文件大小或上传分片配置不合法")
	}
	if size > maxObjectBytes {
		return 0, 0, apperr.New(apperr.TooLarge, "upload_too_large", "文件超过对象存储的 5 TB 限制")
	}
	minimum := (size + 9999) / 10000
	partSize = max(configuredPart, (minimum+mebibyte-1)/mebibyte*mebibyte)
	return partSize, int((size + partSize - 1) / partSize), nil
}

func uploadFilename(filename string) (title, ext string) {
	title = path.Base(strings.ReplaceAll(strings.TrimSpace(filename), "\\", "/"))
	ext = strings.TrimPrefix(strings.ToLower(path.Ext(title)), ".")
	if ext == "" || len(ext) > 10 {
		return title, "bin"
	}
	for _, char := range ext {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') {
			return title, "bin"
		}
	}
	return title, ext
}

func (s *Service) StartUpload(ctx context.Context, p workspace.Principal, filename string, size int64, contentType string, projectID *int64) (video Video, plan UploadPlan, err error) {
	partSize, count, err := uploadSizing(size, s.cfg.UploadPartBytes)
	if err != nil {
		return Video{}, UploadPlan{}, err
	}
	title, ext := uploadFilename(filename)
	contentType, _, typeErr := mime.ParseMediaType(contentType)
	if title == "" || title == "." || title == ".." || title == "/" || typeErr != nil || !strings.HasPrefix(contentType, "video/") || s.cfg.PresignTTL < time.Second || s.cfg.PresignTTL > 7*24*time.Hour {
		return Video{}, UploadPlan{}, apperr.New(apperr.Invalid, "invalid_upload", "文件名、媒体类型或上传有效期不合法")
	}
	if s.objects == nil {
		return Video{}, UploadPlan{}, unavailable()
	}
	var row librarydb.Video
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := librarydb.New(tx)
		if projectID != nil {
			if _, err := q.Project(ctx, librarydb.ProjectParams{ID: *projectID, WorkspaceID: p.WorkspaceID}); err != nil {
				return rowError(err)
			}
		}
		var err error
		row, err = q.CreateUploadVideo(ctx, librarydb.CreateUploadVideoParams{WorkspaceID: p.WorkspaceID, Title: title})
		if err != nil {
			return err
		}
		if projectID != nil {
			allowed, err := q.AddVideoToProject(ctx, librarydb.AddVideoToProjectParams{WorkspaceID: p.WorkspaceID, ProjectID: *projectID, VideoID: row.ID})
			if err != nil {
				return err
			}
			if !allowed {
				return notFound()
			}
		}
		return nil
	})
	if err != nil {
		return Video{}, UploadPlan{}, err
	}
	key, uploadID := blob.SourceKey(p.WorkspaceID, row.ID, ext), ""
	defer func() {
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			var cleanupErr error
			if uploadID != "" {
				cleanupErr = s.objects.AbortMultipart(cleanup, key, uploadID)
			}
			if cleanupErr == nil {
				cleanupErr = librarydb.New(s.pool).RemoveUnstartedUpload(cleanup, row.ID)
			}
			err = errors.Join(err, cleanupErr)
		}
	}()
	uploadID, err = s.objects.CreateMultipart(ctx, key, contentType)
	if err != nil {
		return Video{}, UploadPlan{}, err
	}
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := GetForWorker(ctx, tx, row.ID); err != nil {
			return err
		}
		return librarydb.New(tx).SaveUpload(ctx, librarydb.SaveUploadParams{VideoID: row.ID, ObjectKey: key, UploadID: uploadID, SizeBytes: size, PartSize: partSize})
	})
	if err != nil {
		return Video{}, UploadPlan{}, err
	}
	expiresAt := time.Now().Add(s.cfg.PresignTTL)
	parts, err := s.objects.PresignParts(ctx, key, uploadID, count, s.cfg.PresignTTL)
	if err != nil {
		return Video{}, UploadPlan{}, err
	}
	if err := ctx.Err(); err != nil {
		return Video{}, UploadPlan{}, err
	}
	video, err = videoFromRow(row)
	if err != nil {
		return Video{}, UploadPlan{}, err
	}
	return video, UploadPlan{UploadID: uploadID, PartSize: partSize, Parts: parts, ExpiresAt: expiresAt}, nil
}

func validateParts(parts []blob.Part, session librarydb.VideoUpload) error {
	expected := int((session.SizeBytes + session.PartSize - 1) / session.PartSize)
	if len(parts) != expected {
		return apperr.New(apperr.Invalid, "invalid_parts", "上传分片数量不正确")
	}
	ordered := slices.Clone(parts)
	slices.SortFunc(ordered, func(a, b blob.Part) int { return cmp.Compare(a.N, b.N) })
	for i, part := range ordered {
		if part.N != i+1 || part.ETag == "" {
			return apperr.New(apperr.Invalid, "invalid_parts", "上传分片序号或 ETag 不合法")
		}
	}
	return nil
}

func (s *Service) completeObject(ctx context.Context, session librarydb.VideoUpload, parts []blob.Part) error {
	info, err := s.objects.Head(ctx, session.ObjectKey)
	if errors.Is(err, blob.ErrNotFound) {
		completeErr := s.objects.CompleteMultipart(ctx, session.ObjectKey, session.UploadID, parts)
		info, err = s.objects.Head(ctx, session.ObjectKey)
		if err != nil {
			return errors.Join(completeErr, err)
		}
	} else if err != nil {
		return err
	}
	if info.Size != session.SizeBytes {
		return apperr.New(apperr.Invalid, "upload_size_mismatch", "实际上传大小与登记大小不一致")
	}
	return nil
}

func (s *Service) CompleteUpload(ctx context.Context, p workspace.Principal, videoID int64, uploadID string, parts []blob.Part) (Video, error) {
	video, err := s.Get(ctx, p, videoID)
	if err != nil {
		return Video{}, err
	}
	session, err := librarydb.New(s.pool).Upload(ctx, librarydb.UploadParams{VideoID: videoID, WorkspaceID: p.WorkspaceID})
	if err != nil {
		return Video{}, rowError(err)
	}
	if uploadID == "" || uploadID != session.UploadID {
		return Video{}, apperr.New(apperr.Invalid, "invalid_upload", "上传凭据不匹配")
	}
	if err := validateParts(parts, session); err != nil {
		return Video{}, err
	}
	if video.SourceKind != "upload" || (video.Status != "uploading" && video.SourceKey != session.ObjectKey) {
		return Video{}, apperr.New(apperr.Conflict, "invalid_upload_state", "当前状态不能完成上传")
	}
	if s.objects == nil {
		return Video{}, unavailable()
	}
	if err := s.completeObject(ctx, session, parts); err != nil {
		return Video{}, err
	}
	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		row, err := librarydb.New(tx).VideoIncludingDeleted(ctx, librarydb.VideoIncludingDeletedParams{ID: videoID, WorkspaceID: p.WorkspaceID})
		if err != nil {
			return rowError(err)
		}
		if row.DeletedAt.Valid {
			return notFound()
		}
		if row.Status != "uploading" && (!row.SourceKey.Valid || row.SourceKey.String != session.ObjectKey) {
			return apperr.New(apperr.Conflict, "invalid_upload_state", "当前状态不能完成上传")
		}
		if row.SourceKey.Valid && row.SourceKey.String != session.ObjectKey {
			return apperr.New(apperr.Conflict, "invalid_upload_state", "原片路径不匹配")
		}
		if row.SourceKey.Valid {
			return nil
		}
		return UpdateSource(ctx, tx, videoID, session.ObjectKey)
	})
	if err != nil {
		return Video{}, err
	}
	video, err = s.Get(ctx, p, videoID)
	if err != nil {
		return Video{}, err
	}
	if video.Status != "uploading" {
		return video, nil
	}
	if s.starter == nil {
		return Video{}, unavailable()
	}
	if err := s.starter.Start(ctx, videoID); err != nil {
		return Video{}, err
	}
	return s.Get(ctx, p, videoID)
}
