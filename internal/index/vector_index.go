package index

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ozine/videohelper/internal/platform/db"
)

func indexName(channel, spaceID string) string {
	digest := sha256.Sum256([]byte(channel + "\x00" + spaceID))
	return "emb_hnsw_" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(digest[:]))
}

func VectorIndexReady(ctx context.Context, q db.Querier, channel, spaceID string) (bool, error) {
	if err := literal(channel); err != nil {
		return false, err
	}
	if err := literal(spaceID); err != nil {
		return false, err
	}
	var ready bool
	err := q.QueryRow(ctx, `SELECT EXISTS (
SELECT 1 FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid
WHERE i.indrelid='embedding'::regclass AND c.relname=$1 AND i.indisvalid AND i.indisready)`, indexName(channel, spaceID)).Scan(&ready)
	return ready, err
}

func EnsureVectorIndex(ctx context.Context, pool *pgxpool.Pool, channel string, sp VectorSpace) error {
	if _, err := sp.operator(); err != nil {
		return err
	}
	if err := literal(channel); err != nil {
		return err
	}
	ready, err := VectorIndexReady(ctx, pool, channel, sp.ID)
	if err != nil || ready {
		return err
	}
	name := pgx.Identifier{indexName(channel, sp.ID)}.Sanitize()
	if _, err := pool.Exec(ctx, "DROP INDEX CONCURRENTLY IF EXISTS "+name); err != nil {
		return err
	}
	class := map[string]string{"cosine": "halfvec_cosine_ops", "ip": "halfvec_ip_ops", "l2": "halfvec_l2_ops"}[sp.Metric]
	_, err = pool.Exec(ctx, fmt.Sprintf(`CREATE INDEX CONCURRENTLY IF NOT EXISTS %s ON embedding
USING hnsw ((vec::halfvec(%d)) %s) WHERE channel='%s' AND space_id='%s'`, name, sp.Dim, class, channel, sp.ID))
	return err
}
