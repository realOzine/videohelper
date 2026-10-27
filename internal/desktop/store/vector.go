package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"

	sqlitevec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	"github.com/ozine/videohelper/internal/timeline"
)

var identifier = regexp.MustCompile(`^[a-z0-9@._:-]+$`)

func vectorTable(id string) string {
	sum := sha256.Sum256([]byte(id))
	return fmt.Sprintf("vec_%x", sum[:16])
}

func scanSpace(row scanner) (Space, error) {
	var sp Space
	var docs, queries string
	err := row.Scan(&sp.ID, &sp.Fingerprint, &sp.Dim, &sp.Metric, &sp.Disabled, &docs, &queries)
	if err != nil {
		return sp, missing(err)
	}
	if err := json.Unmarshal([]byte(docs), &sp.DocInputs); err != nil {
		return sp, err
	}
	err = json.Unmarshal([]byte(queries), &sp.QueryInputs)
	return sp, err
}

const spaceColumns = `id,fingerprint,dim,metric,disabled,doc_inputs,query_inputs`

func (s *Store) Space(ctx context.Context, id string) (Space, error) {
	return scanSpace(s.db.QueryRowContext(ctx, "SELECT "+spaceColumns+" FROM embedding_space WHERE id=?", id))
}
func (s *Store) Spaces(ctx context.Context) ([]Space, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+spaceColumns+" FROM embedding_space ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	spaces := []Space{}
	for rows.Next() {
		sp, err := scanSpace(rows)
		if err != nil {
			return nil, err
		}
		spaces = append(spaces, sp)
	}
	return spaces, rows.Err()
}

// RegisterSpace never overwrites an existing identity. A mismatch disables the
// old ID and preserves its rows; use a new versioned ID for the changed model.
func (s *Store) RegisterSpace(ctx context.Context, sp Space) (Space, error) {
	if !identifier.MatchString(sp.ID) || sp.Fingerprint == "" || sp.Dim < 1 || sp.Dim > 4000 || (sp.Metric != "cosine" && sp.Metric != "l2") {
		return Space{}, ErrInvalid
	}
	var result Space
	var conflict bool
	err := s.write(ctx, func(tx *sql.Tx) error {
		old, err := scanSpace(tx.QueryRowContext(ctx, "SELECT "+spaceColumns+" FROM embedding_space WHERE id=?", sp.ID))
		if err == nil {
			if old.Fingerprint != sp.Fingerprint || old.Dim != sp.Dim || old.Metric != sp.Metric {
				if _, err := tx.ExecContext(ctx, "UPDATE embedding_space SET disabled=1 WHERE id=?", sp.ID); err != nil {
					return err
				}
				old.Disabled = true
				conflict = true
			}
			result = old
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		docs, err := json.Marshal(sp.DocInputs)
		if err != nil {
			return err
		}
		queries, err := json.Marshal(sp.QueryInputs)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO embedding_space("+spaceColumns+") VALUES(?,?,?,?,?,?,?)", sp.ID, sp.Fingerprint, sp.Dim, sp.Metric, sp.Disabled, string(docs), string(queries)); err != nil {
			return err
		}
		table := vectorTable(sp.ID)
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("CREATE VIRTUAL TABLE %s USING vec0(embedding float[%d] distance_metric=%s)", table, sp.Dim, sp.Metric)); err != nil {
			return err
		}
		// Deleting metadata, including FK cascades, must remove the vec0 row too.
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("CREATE TRIGGER %s_delete AFTER DELETE ON embedding WHEN OLD.space_id='%s' BEGIN DELETE FROM %s WHERE rowid=OLD.id; END", table, sp.ID, table)); err != nil {
			return err
		}
		result = sp
		return nil
	})
	if err == nil && conflict {
		err = ErrConflict
	}
	return result, err
}

func validSpan(span timeline.Span) bool {
	return span.StartMs >= 0 && span.EndMs > span.StartMs && span.EndMs <= math.MaxInt32
}
func vectorBytes(vec []float32, sp Space) ([]byte, error) {
	if len(vec) != sp.Dim || sp.Disabled {
		return nil, ErrInvalid
	}
	var norm float64
	for _, v := range vec {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return nil, ErrInvalid
		}
		norm += float64(v) * float64(v)
	}
	if sp.Metric == "cosine" && norm == 0 {
		return nil, ErrInvalid
	}
	return sqlitevec.SerializeFloat32(vec)
}

func (t *Tx) PutVectors(ctx context.Context, rows []VectorRow) error {
	for _, row := range rows {
		if !validSpan(row.Span) || !identifier.MatchString(row.Seg) || (row.Channel != "visual" && row.Channel != "speech" && row.Channel != "audio") {
			return ErrInvalid
		}
		sp, err := scanSpace(t.tx.QueryRowContext(ctx, "SELECT "+spaceColumns+" FROM embedding_space WHERE id=?", row.SpaceID))
		if err != nil {
			return err
		}
		if row.Fingerprint != sp.Fingerprint || sp.Disabled {
			return ErrConflict
		}
		blob, err := vectorBytes(row.Vec, sp)
		if err != nil {
			return err
		}
		var visible int64
		if err := t.tx.QueryRowContext(ctx, "SELECT id FROM video WHERE id=? AND deleted_at IS NULL", row.VideoID).Scan(&visible); err != nil {
			return missing(err)
		}
		var id int64
		err = t.tx.QueryRowContext(ctx, `INSERT INTO embedding(video_id,space_id,channel,seg,start_ms,end_ms) VALUES(?,?,?,?,?,?) ON CONFLICT(video_id,space_id,channel,seg,start_ms) DO UPDATE SET end_ms=excluded.end_ms RETURNING id`, row.VideoID, row.SpaceID, row.Channel, row.Seg, row.Span.StartMs, row.Span.EndMs).Scan(&id)
		if err != nil {
			return err
		}
		table := vectorTable(sp.ID)
		if _, err := t.tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE rowid=?", id); err != nil {
			return err
		}
		if _, err := t.tx.ExecContext(ctx, "INSERT INTO "+table+"(rowid,embedding) VALUES(?,?)", id, blob); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) PutVectors(ctx context.Context, rows []VectorRow) error {
	return s.write(ctx, func(tx *sql.Tx) error { return (&Tx{tx}).PutVectors(ctx, rows) })
}

func (t *Tx) DeleteVectors(ctx context.Context, videoID int64, channel, spaceID string) error {
	query := "DELETE FROM embedding WHERE video_id=?"
	args := []any{videoID}
	if channel != "" {
		query += " AND channel=?"
		args = append(args, channel)
	}
	if spaceID != "" {
		query += " AND space_id=?"
		args = append(args, spaceID)
	}
	_, err := t.tx.ExecContext(ctx, query, args...)
	return err
}

func (s *Store) SearchVectors(ctx context.Context, q VectorQuery) ([]VectorHit, error) {
	if q.K <= 0 || q.K > 4096 || !identifier.MatchString(q.Channel) || !identifier.MatchString(q.Seg) || (q.TimeRange != nil && !validSpan(*q.TimeRange)) {
		return nil, ErrInvalid
	}
	if q.VideoIDs != nil && len(q.VideoIDs) == 0 {
		return []VectorHit{}, nil
	}
	var hits = []VectorHit{}
	// The identity lookup and KNN run under one transaction, so disabling a space
	// cannot race a search or cross a model fingerprint boundary.
	err := s.write(ctx, func(tx *sql.Tx) error {
		sp, err := scanSpace(tx.QueryRowContext(ctx, "SELECT "+spaceColumns+" FROM embedding_space WHERE id=?", q.SpaceID))
		if err != nil {
			return err
		}
		if sp.Disabled || sp.Fingerprint != q.Fingerprint {
			return ErrConflict
		}
		blob, err := vectorBytes(q.Vec, sp)
		if err != nil {
			return err
		}
		filter := `SELECT e.id FROM embedding e JOIN video v ON v.id=e.video_id WHERE e.space_id=? AND e.channel=? AND e.seg=? AND v.status='ready' AND v.deleted_at IS NULL`
		args := []any{blob, q.K, q.SpaceID, q.Channel, q.Seg}
		if q.VideoIDs != nil {
			filter += " AND e.video_id IN (" + placeholders(len(q.VideoIDs)) + ")"
			for _, id := range q.VideoIDs {
				args = append(args, id)
			}
		}
		if q.ProjectID != nil {
			filter += " AND EXISTS(SELECT 1 FROM project_video p WHERE p.video_id=e.video_id AND p.project_id=?)"
			args = append(args, *q.ProjectID)
		}
		if q.TimeRange != nil {
			filter += " AND e.start_ms<? AND e.end_ms>?"
			args = append(args, q.TimeRange.EndMs, q.TimeRange.StartMs)
		}
		query := `WITH hits AS MATERIALIZED (SELECT rowid,distance FROM ` + vectorTable(sp.ID) + ` WHERE embedding MATCH ? AND k=? AND rowid IN (` + filter + `)) SELECT e.video_id,e.start_ms,e.end_ms,h.distance FROM hits h JOIN embedding e ON e.id=h.rowid ORDER BY h.distance,e.video_id,e.start_ms,e.end_ms`
		rows, err := tx.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var hit VectorHit
			if err := rows.Scan(&hit.VideoID, &hit.Span.StartMs, &hit.Span.EndMs, &hit.Distance); err != nil {
				return err
			}
			hit.Rank = len(hits) + 1
			hits = append(hits, hit)
		}
		return rows.Err()
	})
	return hits, err
}

// VectorCount reports coverage for one versioned channel without loading blobs.
func (s *Store) VectorCount(ctx context.Context, videoID int64, channel, spaceID string) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM embedding WHERE video_id=? AND channel=? AND space_id=?", videoID, channel, spaceID).Scan(&count)
	return count, err
}

func validText(s string) bool { return strings.TrimSpace(s) != "" }
