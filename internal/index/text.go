package index

import (
	"context"
	"encoding/json"
	"errors"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/index/indexdb"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/timeline"
)

type Word struct {
	W string `json:"w"`
	S int    `json:"s"`
	E int    `json:"e"`
}

type TextSpan struct {
	VideoID, WorkspaceID int64
	Kind                 string
	Span                 timeline.Span
	Text                 string
	Words                []Word
	Model                string
}

// ReplaceSpeech belongs in the caller's output-and-stage-state transaction.
func ReplaceSpeech(ctx context.Context, tx pgx.Tx, workspaceID, videoID int64, model string, sents []TextSpan) error {
	q := indexdb.New(tx)
	if _, err := q.LockSpeechVideo(ctx, indexdb.LockSpeechVideoParams{ID: videoID, WorkspaceID: workspaceID}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperr.New(apperr.NotFound, "not_found", "视频不存在")
		}
		return err
	}
	if err := q.DeleteSpeechText(ctx, indexdb.DeleteSpeechTextParams{VideoID: videoID, WorkspaceID: workspaceID}); err != nil {
		return err
	}
	if err := q.DeleteSpeechVectors(ctx, indexdb.DeleteSpeechVectorsParams{VideoID: videoID, WorkspaceID: workspaceID}); err != nil {
		return err
	}
	for _, sent := range sents {
		if err := validSpan(sent.Span); err != nil {
			return err
		}
		var words []byte
		if sent.Words != nil {
			var err error
			words, err = json.Marshal(sent.Words)
			if err != nil {
				return err
			}
		}
		if err := q.InsertSpeech(ctx, indexdb.InsertSpeechParams{
			VideoID: videoID, WorkspaceID: workspaceID, StartMs: int32(sent.Span.StartMs), EndMs: int32(sent.Span.EndMs),
			Text: sent.Text, Words: words, Model: model,
		}); err != nil {
			return err
		}
	}
	return nil
}

func TextRange(ctx context.Context, q db.Querier, workspaceID, videoID int64, span timeline.Span) ([]TextSpan, error) {
	if err := validSpan(span); err != nil {
		return nil, err
	}
	rows, err := indexdb.New(q).TextRange(ctx, indexdb.TextRangeParams{
		WorkspaceID: workspaceID, VideoID: videoID, RangeEndMs: int32(span.EndMs), RangeStartMs: int32(span.StartMs),
	})
	if err != nil {
		return nil, err
	}
	return textSpans(rows)
}

func SpeechSentences(ctx context.Context, q db.Querier, videoID int64) ([]TextSpan, error) {
	rows, err := indexdb.New(q).SpeechSentences(ctx, videoID)
	if err != nil {
		return nil, err
	}
	return textSpans(rows)
}

func textSpans(rows []indexdb.TextSpan) ([]TextSpan, error) {
	spans := make([]TextSpan, 0, len(rows))
	for _, row := range rows {
		span := TextSpan{
			VideoID: row.VideoID, WorkspaceID: row.WorkspaceID, Kind: row.Kind, Model: row.Model,
			Span: timeline.Span{StartMs: int(row.StartMs), EndMs: int(row.EndMs)}, Text: row.Text,
		}
		if row.Words != nil {
			if err := json.Unmarshal(row.Words, &span.Words); err != nil {
				return nil, err
			}
		}
		spans = append(spans, span)
	}
	return spans, nil
}

func validSpan(span timeline.Span) error {
	if span.StartMs < 0 || span.EndMs <= span.StartMs || span.EndMs > math.MaxInt32 {
		return apperr.New(apperr.Invalid, "invalid_argument", "时间范围超出数据库毫秒范围")
	}
	return nil
}
