package stages

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/pipeline"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/timeline"
)

type asrProgress struct {
	Next      int      `json:"next"`
	Total     int      `json:"total"`
	ChunkMs   int      `json:"chunk_ms"`
	OverlapMs int      `json:"overlap_ms"`
	Keys      []string `json:"keys"`
}

func (h *Handlers) ASR(ctx context.Context, sc *pipeline.StageCtx) (pipeline.Commit, error) {
	opts := sc.Def.Options
	model, endpoint, ok := h.registry.Transcriber()
	if !ok || model.ID != opts.Transcriber {
		return nil, apperr.New(apperr.Unavailable, "asr_unavailable", "转录模型暂时不可用")
	}
	if model.Fingerprint != opts.Fingerprint {
		return nil, apperr.New(apperr.Invalid, "asr_changed", "转录模型版本已改变")
	}
	progress := asrProgress{ChunkMs: h.cfg.ML.TranscribeChunkMS, OverlapMs: h.cfg.ML.TranscribeOverlapMS, Keys: []string{}}
	if len(sc.Progress) > 0 {
		if err := json.Unmarshal(sc.Progress, &progress); err != nil {
			return nil, err
		}
	}
	if progress.ChunkMs <= 0 || progress.OverlapMs < 0 || progress.ChunkMs+2*progress.OverlapMs > opts.MaxWindowMs {
		return nil, apperr.New(apperr.Invalid, "invalid_asr_window", "转录分段和重叠长度超出模型窗口")
	}
	duration := *sc.Video.DurationMs
	total := (duration + progress.ChunkMs - 1) / progress.ChunkMs
	if len(progress.Keys) > total || progress.Next != len(progress.Keys) || (progress.Total != 0 && progress.Total != total) {
		return nil, apperr.New(apperr.Invalid, "invalid_checkpoint", "转录断点与当前视频不一致")
	}
	progress.Total = total
	var sentences []index.TextSpan
	previousWord := 0
	for i := 0; i < total; i++ {
		start, end := i*progress.ChunkMs, min((i+1)*progress.ChunkMs, duration)
		request := mlclient.TranscribeRequest{Model: model.ID, URI: h.objects.URI(sc.Video.NormalizedVideoKey), Language: "zh", StartMs: max(0, start-progress.OverlapMs), EndMs: min(duration, end+progress.OverlapMs)}
		var response mlclient.TranscribeResponse
		if i < len(progress.Keys) {
			if err := h.readTranscript(ctx, progress.Keys[i], &response); err != nil {
				return nil, err
			}
		} else {
			var err error
			response, err = h.ml.Transcribe(ctx, endpoint, request)
			if err != nil {
				return nil, err
			}
			if response.Model != opts.Transcriber || response.Fingerprint != opts.Fingerprint {
				return nil, apperr.New(apperr.Invalid, "asr_changed", "转录结果与计划中的模型不一致")
			}
			encoded, err := json.Marshal(response)
			if err != nil {
				return nil, err
			}
			key := blob.RunRawKey(sc.Video.WorkspaceID, sc.Video.ID, sc.Payload.Version, sc.Attempt, fmt.Sprintf("asr-%s/chunk-%d.json", model.ID, i))
			if err := h.objects.Put(ctx, key, bytes.NewReader(encoded), "application/json"); err != nil {
				return nil, err
			}
			progress.Keys = append(progress.Keys, key)
			progress.Next = len(progress.Keys)
			if err := sc.SaveProgress(ctx, progress, nil); err != nil {
				return nil, err
			}
		}
		if response.Model != opts.Transcriber || response.Fingerprint != opts.Fingerprint {
			return nil, apperr.New(apperr.Invalid, "asr_changed", "转录断点与计划中的模型不一致")
		}
		if err := mlclient.ValidateTranscript(request, response, model); err != nil {
			return nil, err
		}
		for _, sentence := range response.Sentences {
			// shortcut: sentences belong to the chunk containing their start; increase overlap for speech longer than the overlap window.
			if sentence.StartMs < start || sentence.StartMs >= end {
				continue
			}
			if len(sentences) > 0 && sentence.StartMs < sentences[len(sentences)-1].Span.StartMs {
				return nil, apperr.New(apperr.Invalid, "asr_overlap", "相邻转录窗口的句子时间顺序倒退")
			}
			words := make([]index.Word, len(sentence.Words))
			for i, word := range sentence.Words {
				if word.S < previousWord {
					return nil, apperr.New(apperr.Invalid, "asr_overlap", "相邻转录窗口的词语时间顺序倒退")
				}
				previousWord = word.S
				words[i] = index.Word{W: word.W, S: word.S, E: word.E}
			}
			sentences = append(sentences, index.TextSpan{WorkspaceID: sc.Video.WorkspaceID, VideoID: sc.Video.ID, Kind: "speech", Span: timeline.Span{StartMs: sentence.StartMs, EndMs: sentence.EndMs}, Text: sentence.Text, Words: words, Model: model.ID})
		}
	}
	return func(ctx context.Context, tx pgx.Tx) error {
		return index.ReplaceSpeech(ctx, tx, sc.Video.WorkspaceID, sc.Video.ID, model.ID, sentences)
	}, nil
}

func (h *Handlers) readTranscript(ctx context.Context, key string, out *mlclient.TranscribeResponse) error {
	body, err := h.objects.Open(ctx, key)
	if err != nil {
		return err
	}
	defer body.Close()
	return json.NewDecoder(io.LimitReader(body, 32<<20)).Decode(out)
}
