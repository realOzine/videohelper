package index

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/timeline"
	"github.com/pgvector/pgvector-go"
)

type VectorSpace struct {
	ID     string
	Dim    int
	Metric string
}

type VectorRow struct {
	VideoID, WorkspaceID  int64
	Channel, SpaceID, Seg string
	Span                  timeline.Span
	Vec                   []float32
}

type VectorQuery struct {
	WorkspaceID int64
	VideoIDs    []int64
	Channel     string
	Seg         string
	Space       VectorSpace
	Vec         []float32
	TimeRange   *timeline.Span
	K           int
}

type VectorHit struct {
	VideoID  int64
	Span     timeline.Span
	Distance float32
	Rank     int
}

var literalPattern = regexp.MustCompile(`^[a-z0-9@._-]+$`)

func literal(value string) error {
	if !literalPattern.MatchString(value) {
		return apperr.New(apperr.Invalid, "invalid_argument", "向量元数据格式不合法")
	}
	return nil
}

func (sp VectorSpace) operator() (string, error) {
	if err := literal(sp.ID); err != nil {
		return "", err
	}
	if sp.Dim < 1 || sp.Dim > 4000 {
		return "", apperr.New(apperr.Invalid, "invalid_argument", "向量维度必须在 1 到 4000 之间")
	}
	switch sp.Metric {
	case "cosine":
		return "<=>", nil
	case "ip":
		return "<#>", nil
	case "l2":
		return "<->", nil
	default:
		return "", apperr.New(apperr.Invalid, "invalid_argument", "向量距离类型不合法")
	}
}

func validVector(vec []float32, sp VectorSpace) error {
	if len(vec) != sp.Dim {
		return apperr.New(apperr.Invalid, "invalid_argument", "向量维度与空间不一致")
	}
	nonzero := false
	for _, v := range vec {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) || math.Abs(float64(v)) > 65504 {
			return apperr.New(apperr.Invalid, "invalid_argument", "向量包含 halfvec 无法表示的值")
		}
		// Values at or below half the smallest halfvec subnormal round to zero.
		nonzero = nonzero || math.Abs(float64(v)) > 1.0/(1<<25)
	}
	if sp.Metric == "cosine" && !nonzero {
		return apperr.New(apperr.Invalid, "invalid_argument", "余弦向量不能全为零")
	}
	return nil
}

func UpsertVectors(ctx context.Context, q db.Querier, rows []VectorRow) error {
	spaces := make(map[string]VectorSpace)
	for _, row := range rows {
		for _, value := range []string{row.Channel, row.SpaceID, row.Seg} {
			if err := literal(value); err != nil {
				return err
			}
		}
		sp, ok := spaces[row.SpaceID]
		if !ok {
			sp.ID = row.SpaceID
			var disabled bool
			if err := q.QueryRow(ctx, "SELECT dim, metric, disabled FROM embedding_space WHERE id=$1", row.SpaceID).Scan(&sp.Dim, &sp.Metric, &disabled); err != nil {
				return err
			}
			if disabled {
				return apperr.New(apperr.Unavailable, "unavailable", "向量空间已停用")
			}
			spaces[row.SpaceID] = sp
		}
		if err := validVector(row.Vec, sp); err != nil {
			return err
		}
		_, err := q.Exec(ctx, `INSERT INTO embedding(video_id,workspace_id,channel,space_id,seg,start_ms,end_ms,vec)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (video_id,channel,space_id,seg,start_ms) DO UPDATE SET workspace_id=EXCLUDED.workspace_id,end_ms=EXCLUDED.end_ms,vec=EXCLUDED.vec`,
			row.VideoID, row.WorkspaceID, row.Channel, row.SpaceID, row.Seg, row.Span.StartMs, row.Span.EndMs, pgvector.NewHalfVector(row.Vec))
		if err != nil {
			return err
		}
	}
	return nil
}

func SearchVectors(ctx context.Context, pool *pgxpool.Pool, vq VectorQuery) ([]VectorHit, error) {
	sql, args, err := vectorSQL(vq)
	if err != nil {
		return nil, err
	}
	hits := make([]VectorHit, 0)
	if vq.VideoIDs != nil && len(vq.VideoIDs) == 0 {
		return hits, nil
	}
	err = db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL hnsw.ef_search=100; SET LOCAL hnsw.iterative_scan=relaxed_order"); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, sql, args...)
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

func vectorSQL(vq VectorQuery) (string, []any, error) {
	op, err := vq.Space.operator()
	if err != nil {
		return "", nil, err
	}
	if err := literal(vq.Channel); err != nil {
		return "", nil, err
	}
	if err := literal(vq.Seg); err != nil {
		return "", nil, err
	}
	if err := validVector(vq.Vec, vq.Space); err != nil {
		return "", nil, err
	}
	if vq.K <= 0 || (vq.TimeRange != nil && (vq.TimeRange.StartMs < 0 || vq.TimeRange.EndMs <= vq.TimeRange.StartMs)) {
		return "", nil, apperr.New(apperr.Invalid, "invalid_argument", "召回数量或时间范围不合法")
	}
	distance := fmt.Sprintf("e.vec::halfvec(%d) %s $1", vq.Space.Dim, op)
	sql := fmt.Sprintf(`WITH hits AS MATERIALIZED (
SELECT e.video_id,e.start_ms,e.end_ms,%s AS distance
FROM embedding e JOIN video v ON v.id=e.video_id AND v.workspace_id=e.workspace_id
WHERE e.channel='%s' AND e.space_id='%s' AND e.seg='%s' AND e.workspace_id=$2
AND v.status='ready' AND v.deleted_at IS NULL`, distance, vq.Channel, vq.Space.ID, vq.Seg)
	args := []any{pgvector.NewHalfVector(vq.Vec), vq.WorkspaceID, vq.K}
	if vq.VideoIDs != nil {
		args = append(args, vq.VideoIDs)
		sql += " AND e.video_id=ANY($" + strconv.Itoa(len(args)) + ")"
	}
	if vq.TimeRange != nil {
		args = append(args, vq.TimeRange.EndMs, vq.TimeRange.StartMs)
		sql += fmt.Sprintf(" AND e.start_ms<$%d AND e.end_ms>$%d", len(args)-1, len(args))
	}
	sql += " ORDER BY " + distance + " LIMIT $3) SELECT * FROM hits ORDER BY distance,video_id,start_ms,end_ms"
	return sql, args, nil
}
