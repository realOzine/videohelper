package stages

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/media"
	"github.com/ozine/videohelper/internal/pipeline"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/timeline"
	"github.com/ozine/videohelper/internal/usage"
)

func (h *Handlers) Probe(ctx context.Context, sc *pipeline.StageCtx) (pipeline.Commit, error) {
	url, err := h.objects.PresignGet(ctx, sc.Video.SourceKey, max(h.cfg.PresignTTL, sc.Def.Timeout+time.Minute))
	if err != nil {
		return nil, err
	}
	info, err := media.Probe(ctx, url)
	if err != nil {
		return nil, err
	}
	if info.DurationMs <= 0 || info.DurationMs > math.MaxInt32 || (!info.HasVideo && !info.HasAudio) {
		return nil, apperr.New(apperr.Invalid, "invalid_media", "无法识别的视频文件或时长超出范围")
	}
	return func(ctx context.Context, tx pgx.Tx) error {
		return library.UpdateProbe(ctx, tx, sc.Video.ID, info.DurationMs, info.HasVideo, info.HasAudio)
	}, nil
}

func (h *Handlers) PrepareMedia(ctx context.Context, sc *pipeline.StageCtx) (pipeline.Commit, error) {
	if err := os.MkdirAll(h.cfg.Media.TmpDir, 0700); err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp(h.cfg.Media.TmpDir, "prepare-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	source, err := h.objects.PresignGet(ctx, sc.Video.SourceKey, max(h.cfg.PresignTTL, sc.Def.Timeout+time.Minute))
	if err != nil {
		return nil, err
	}
	normalized := filepath.Join(directory, "normalized_video.mp4")
	if err := media.Transcode(ctx, source, normalized, media.TranscodeOptions{MaxHeight: h.cfg.Media.NormalizedVideoMaxHeight, GOPSeconds: h.cfg.Media.NormalizedVideoGOPSeconds}); err != nil {
		return nil, err
	}
	layout := timeline.SpriteLayout{TileW: h.cfg.Media.ThumbWidth, TileH: 1, Cols: h.cfg.Media.SpriteCols, Rows: h.cfg.Media.SpriteRows}
	var sprites []string
	if sc.Video.HasVideo != nil && *sc.Video.HasVideo {
		sprites, layout, err = media.Sprites(ctx, normalized, filepath.Join(directory, "sprites"), timeline.Cells(*sc.Video.DurationMs), h.cfg.Media.ThumbWidth, h.cfg.Media.SpriteCols, h.cfg.Media.SpriteRows)
		if err != nil {
			return nil, err
		}
	}
	key := blob.RunNormalizedVideoKey(sc.Video.WorkspaceID, sc.Video.ID, sc.Payload.Version, sc.Attempt)
	if err := h.putFile(ctx, key, normalized, "video/mp4"); err != nil {
		return nil, err
	}
	for i, file := range sprites {
		key := blob.RunSpriteKey(sc.Video.WorkspaceID, sc.Video.ID, sc.Payload.Version, sc.Attempt, i)
		if err := h.putFile(ctx, key, file, "image/jpeg"); err != nil {
			return nil, err
		}
	}
	return func(ctx context.Context, tx pgx.Tx) error {
		return library.UpdatePreparedMedia(ctx, tx, sc.Video.ID, key, layout)
	}, nil
}

func (h *Handlers) putFile(ctx context.Context, key, path, contentType string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() > 5<<30 {
		_, err = h.objects.PutStream(ctx, key, file, info.Size())
		return err
	}
	return h.objects.Put(ctx, key, file, contentType)
}

func (h *Handlers) Finalize(_ context.Context, sc *pipeline.StageCtx) (pipeline.Commit, error) {
	return func(ctx context.Context, tx pgx.Tx) error {
		if err := library.SetStatus(ctx, tx, sc.Video.ID, "ready", ""); err != nil {
			return err
		}
		return usage.RecordIndexed(ctx, tx, sc.Video.WorkspaceID, sc.Video.ID, *sc.Video.DurationMs)
	}, nil
}
