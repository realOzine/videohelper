package store

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/ozine/videohelper/internal/timeline"
)

func (t *Tx) ReplaceSentences(ctx context.Context, videoID int64, sentences []Sentence) error {
	var visible int64
	if err := t.tx.QueryRowContext(ctx, "SELECT id FROM video WHERE id=? AND deleted_at IS NULL", videoID).Scan(&visible); err != nil {
		return missing(err)
	}
	if _, err := t.tx.ExecContext(ctx, "DELETE FROM sentence WHERE video_id=?", videoID); err != nil {
		return err
	}
	last := -1
	for _, sentence := range sentences {
		if !validSpan(timeline.Span{StartMs: sentence.StartMs, EndMs: sentence.EndMs}) || !validText(sentence.Text) || sentence.Model == "" || sentence.StartMs <= last {
			return ErrInvalid
		}
		last = sentence.StartMs
		previous := sentence.StartMs
		for _, word := range sentence.Words {
			if word.S < previous || word.E < word.S || word.E > sentence.EndMs {
				return ErrInvalid
			}
			previous = word.S
		}
		words, err := json.Marshal(sentence.Words)
		if err != nil {
			return err
		}
		if _, err := t.tx.ExecContext(ctx, "INSERT INTO sentence(video_id,start_ms,end_ms,text,words,model) VALUES(?,?,?,?,?,?)", videoID, sentence.StartMs, sentence.EndMs, sentence.Text, string(words), sentence.Model); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ReplaceSentences(ctx context.Context, videoID int64, sentences []Sentence) error {
	return s.write(ctx, func(tx *sql.Tx) error { return (&Tx{tx}).ReplaceSentences(ctx, videoID, sentences) })
}

func (s *Store) Sentences(ctx context.Context, videoID int64, span *timeline.Span) ([]Sentence, error) {
	if span != nil && !validSpan(*span) {
		return nil, ErrInvalid
	}
	query := `SELECT s.video_id,s.start_ms,s.end_ms,s.text,s.words,s.model FROM sentence s JOIN video v ON v.id=s.video_id WHERE s.video_id=? AND v.deleted_at IS NULL`
	args := []any{videoID}
	if span != nil {
		query += " AND s.start_ms<? AND s.end_ms>?"
		args = append(args, span.EndMs, span.StartMs)
	}
	rows, err := s.db.QueryContext(ctx, query+" ORDER BY s.start_ms", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sentences := []Sentence{}
	for rows.Next() {
		sentence := Sentence{Kind: "speech"}
		var words string
		if err := rows.Scan(&sentence.VideoID, &sentence.StartMs, &sentence.EndMs, &sentence.Text, &words, &sentence.Model); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(words), &sentence.Words); err != nil {
			return nil, err
		}
		sentences = append(sentences, sentence)
	}
	return sentences, rows.Err()
}
