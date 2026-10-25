package stages

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/pipeline"
	"github.com/ozine/videohelper/internal/timeline"
)

type embedProgress struct {
	Next  int `json:"next"`
	Total int `json:"total"`
}

func (h *Handlers) Embed(ctx context.Context, sc *pipeline.StageCtx) (pipeline.Commit, error) {
	opts := sc.Def.Options
	sp, ok := h.registry.Get(opts.Space.ID)
	if !ok || sp.Disabled || sp.Fingerprint != opts.Space.Fingerprint || sp.Dim != opts.Space.Dim || sp.Metric != opts.Space.Metric {
		return nil, apperr.New(apperr.Unavailable, "space_changed", "向量空间不可用或版本已改变")
	}
	var inputs []mlclient.EmbedInput
	var spans []timeline.Span
	switch opts.Channel {
	case "visual":
		spans = timeline.Cells(*sc.Video.DurationMs)
		uri := h.objects.URI(sc.Video.NormalizedVideoKey)
		for _, span := range spans {
			inputs = append(inputs, mlclient.EmbedInput{Video: &mlclient.MediaRef{URI: uri, StartMs: span.StartMs, EndMs: span.EndMs}})
		}
	case "speech":
		rows, err := index.SpeechSentences(ctx, h.pool, sc.Video.ID)
		if err != nil {
			return nil, err
		}
		original := make([]timeline.Sentence, 0, len(rows))
		for _, row := range rows {
			original = append(original, timeline.Sentence{Span: row.Span, Text: row.Text})
		}
		for _, sentence := range timeline.MergeSentences(original, 8, 120) {
			spans = append(spans, sentence.Span)
			inputs = append(inputs, mlclient.EmbedInput{Text: sentence.Text})
		}
	default:
		return nil, apperr.New(apperr.Invalid, "unsupported_channel", "当前只启用画面与转录语义通道")
	}
	progress := embedProgress{Total: len(inputs)}
	if len(sc.Progress) > 0 {
		if err := json.Unmarshal(sc.Progress, &progress); err != nil {
			return nil, err
		}
		if progress.Next < 0 || progress.Next > len(inputs) || progress.Total != len(inputs) {
			return nil, apperr.New(apperr.Invalid, "invalid_checkpoint", "编码断点与当前切片不一致")
		}
	}
	batch := min(h.cfg.ML.EmbedBatch, sp.MaxBatch)
	if batch <= 0 {
		return nil, apperr.New(apperr.Invalid, "invalid_batch", "编码批量必须为正数")
	}
	for start := progress.Next; start < len(inputs); start += batch {
		end := min(start+batch, len(inputs))
		response, err := h.ml.Embed(ctx, mlclient.EmbedRequest{Space: sp.ID, Role: "document", Inputs: inputs[start:end]})
		if err != nil {
			return nil, err
		}
		if response.Fingerprint != opts.Space.Fingerprint || response.Dim != opts.Space.Dim || len(response.Vectors) != end-start {
			return nil, apperr.New(apperr.Invalid, "space_changed", "编码结果与计划中的向量空间不一致")
		}
		rows := make([]index.VectorRow, 0, end-start)
		for i, vector := range response.Vectors {
			rows = append(rows, index.VectorRow{WorkspaceID: sc.Video.WorkspaceID, VideoID: sc.Video.ID, Channel: opts.Channel, SpaceID: sp.ID, Seg: opts.Seg, Span: spans[start+i], Vec: vector})
		}
		progress.Next = end
		if err := sc.SaveProgress(ctx, progress, func(ctx context.Context, tx pgx.Tx) error { return index.UpsertVectors(ctx, tx, rows) }); err != nil {
			return nil, err
		}
	}
	return nil, nil
}
