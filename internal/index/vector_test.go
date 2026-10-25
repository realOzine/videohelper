package index

import (
	"context"
	"database/sql"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/timeline"
	"github.com/ozine/videohelper/migrations"
	"github.com/pgvector/pgvector-go"
)

func TestVectorMetadataValidation(t *testing.T) {
	query := VectorQuery{Channel: "visual", Seg: "grid@v1", Space: VectorSpace{ID: "model@r1", Dim: 3, Metric: "cosine"}, Vec: []float32{1, 0, 0}, K: 10}
	for _, change := range []func(*VectorQuery){
		func(q *VectorQuery) { q.Channel = "visual' OR true--" },
		func(q *VectorQuery) { q.Seg = "grid@v1'; DROP TABLE video;--" },
		func(q *VectorQuery) { q.Space.ID = "model'" },
		func(q *VectorQuery) { q.Space.Dim = 4001 },
		func(q *VectorQuery) { q.Space.Metric = "cosine);drop" },
		func(q *VectorQuery) { q.Vec = []float32{1, 2} },
		func(q *VectorQuery) { q.Vec[0] = float32(math.NaN()) },
		func(q *VectorQuery) { q.Vec[0] = 70000 },
		func(q *VectorQuery) { q.Vec = []float32{0, 0, 0} },
		func(q *VectorQuery) { q.Vec = []float32{1e-30, 0, 0} },
		func(q *VectorQuery) { q.K = 0 },
		func(q *VectorQuery) { q.TimeRange = &timeline.Span{StartMs: 10, EndMs: 10} },
	} {
		q := query
		q.Vec = append([]float32(nil), query.Vec...)
		change(&q)
		if _, _, err := vectorSQL(q); err == nil {
			t.Fatalf("invalid query accepted: %+v", q)
		}
	}
	name := indexName("visual", strings.Repeat("a", 100)+"@r1")
	if len(name) > 63 || name == indexName("visual", strings.Repeat("a", 100)+"-r1") || name != indexName("visual", strings.Repeat("a", 100)+"@r1") {
		t.Fatal("index name is truncated, unstable, or loses metadata distinctions")
	}
}

func TestVectorPostgresIntegration(t *testing.T) {
	ctx, pool := indexTestDB(t)
	w1 := insertWorkspace(t, ctx, pool, "first")
	w2 := insertWorkspace(t, ctx, pool, "second")
	v1 := insertVideo(t, ctx, pool, w1, "ready", false)
	v2 := insertVideo(t, ctx, pool, w2, "ready", false)
	v3 := insertVideo(t, ctx, pool, w1, "indexing", false)
	v4 := insertVideo(t, ctx, pool, w1, "ready", true)
	v5 := insertVideo(t, ctx, pool, w1, "ready", false)
	space := VectorSpace{ID: "cosine@r1", Dim: 3, Metric: "cosine"}
	insertSpace(t, ctx, pool, space)
	rows := []VectorRow{
		{v1, w1, "visual", space.ID, "grid@v1", timeline.Span{StartMs: 0, EndMs: 10}, []float32{1, 0, 0}},
		{v1, w1, "visual", space.ID, "grid@v1", timeline.Span{StartMs: 10, EndMs: 20}, []float32{0.8, 0.6, 0}},
		{v5, w1, "visual", space.ID, "grid@v1", timeline.Span{StartMs: 20, EndMs: 30}, []float32{0, 1, 0}},
	}
	for _, hidden := range []struct{ video, workspace int64 }{{v2, w2}, {v3, w1}, {v4, w1}} {
		rows = append(rows, VectorRow{hidden.video, hidden.workspace, "visual", space.ID, "grid@v1", timeline.Span{StartMs: 0, EndMs: 10}, []float32{1, 0, 0}})
	}
	if err := UpsertVectors(ctx, pool, rows); err != nil {
		t.Fatal(err)
	}
	var stored pgvector.HalfVector
	if err := pool.QueryRow(ctx, "SELECT vec FROM embedding WHERE video_id=$1 AND start_ms=10", v1).Scan(&stored); err != nil || math.Abs(float64(stored.Slice()[1]-0.6)) > 0.001 {
		t.Fatalf("halfvec roundtrip = %v, error=%v", stored, err)
	}
	bad := rows[0]
	bad.Vec = []float32{1, 0}
	if err := UpsertVectors(ctx, pool, []VectorRow{bad}); err == nil {
		t.Fatal("dimension mismatch was accepted")
	}
	bad = rows[0]
	bad.WorkspaceID = w2
	if err := UpsertVectors(ctx, pool, []VectorRow{bad}); err == nil {
		t.Fatal("conflict update accepted a foreign workspace")
	}
	updated := rows[1]
	updated.Span.EndMs = 18
	if err := UpsertVectors(ctx, pool, []VectorRow{updated}); err != nil {
		t.Fatal(err)
	}
	var end int
	if err := pool.QueryRow(ctx, "SELECT end_ms FROM embedding WHERE video_id=$1 AND start_ms=10", v1).Scan(&end); err != nil || end != 18 {
		t.Fatalf("upsert end=%d error=%v", end, err)
	}
	if ready, err := VectorIndexReady(ctx, pool, "visual", space.ID); err != nil || ready {
		t.Fatalf("absent index ready=%v error=%v", ready, err)
	}
	if err := EnsureVectorIndex(ctx, pool, "visual", space); err != nil {
		t.Fatal(err)
	}
	if err := EnsureVectorIndex(ctx, pool, "visual", space); err != nil {
		t.Fatalf("idempotent index: %v", err)
	}
	query := VectorQuery{WorkspaceID: w1, Channel: "visual", Seg: "grid@v1", Space: space, Vec: []float32{1, 0, 0}, K: 10}
	hits, err := SearchVectors(ctx, pool, query)
	if err != nil || len(hits) != 3 || hits[0].VideoID != v1 || hits[0].Span.StartMs != 0 || hits[0].Rank != 1 || hits[2].Distance != 1 {
		t.Fatalf("ranked ready/workspace hits=%+v error=%v", hits, err)
	}
	for i, hit := range hits {
		if hit.Rank != i+1 || (i > 0 && hit.Distance < hits[i-1].Distance) {
			t.Fatalf("unordered rank: %+v", hits)
		}
	}
	assertHNSWPlan(t, ctx, pool, query)
	query.VideoIDs = []int64{}
	if hits, err := SearchVectors(ctx, pool, query); err != nil || len(hits) != 0 {
		t.Fatalf("empty scope hits=%v error=%v", hits, err)
	}
	query.VideoIDs = []int64{v2}
	if hits, err := SearchVectors(ctx, pool, query); err != nil || len(hits) != 0 {
		t.Fatalf("foreign scope hits=%v error=%v", hits, err)
	}
	query.VideoIDs = []int64{v1}
	query.TimeRange = &timeline.Span{StartMs: 10, EndMs: 20}
	if hits, err := SearchVectors(ctx, pool, query); err != nil || len(hits) != 1 || hits[0].Span.StartMs != 10 {
		t.Fatalf("half-open time filter hits=%v error=%v", hits, err)
	}
	for _, test := range []struct {
		metric string
		vec    []float32
	}{
		{"l2", []float32{0, 0}},
		{"ip", []float32{1, 0, 0, 0}},
	} {
		t.Run(test.metric, func(t *testing.T) {
			sp := VectorSpace{ID: test.metric + "@r1", Dim: len(test.vec), Metric: test.metric}
			insertSpace(t, ctx, pool, sp)
			row := VectorRow{v1, w1, "visual", sp.ID, "grid@v1", timeline.Span{StartMs: 0, EndMs: 10}, test.vec}
			if err := UpsertVectors(ctx, pool, []VectorRow{row}); err != nil {
				t.Fatal(err)
			}
			if err := EnsureVectorIndex(ctx, pool, "visual", sp); err != nil {
				t.Fatal(err)
			}
			q := VectorQuery{WorkspaceID: w1, Channel: "visual", Seg: "grid@v1", Space: sp, Vec: test.vec, K: 10}
			if hits, err := SearchVectors(ctx, pool, q); err != nil || len(hits) != 1 {
				t.Fatalf("metric=%s hits=%v error=%v", test.metric, hits, err)
			}
			assertHNSWPlan(t, ctx, pool, q)
		})
	}
	t.Run("invalid index repair", func(t *testing.T) {
		sp := VectorSpace{ID: "repair@r1", Dim: 3, Metric: "cosine"}
		insertSpace(t, ctx, pool, sp)
		if _, err := pool.Exec(ctx, "INSERT INTO embedding(video_id,workspace_id,channel,space_id,seg,start_ms,end_ms,vec) VALUES($1,$2,'visual',$3,'grid@v1',0,10,'[1,0]'::halfvec)", v1, w1, sp.ID); err != nil {
			t.Fatal(err)
		}
		if err := EnsureVectorIndex(ctx, pool, "visual", sp); err == nil {
			t.Fatal("invalid dimension unexpectedly built index")
		}
		if ready, err := VectorIndexReady(ctx, pool, "visual", sp.ID); err != nil || ready {
			t.Fatalf("invalid index ready=%v error=%v", ready, err)
		}
		if _, err := pool.Exec(ctx, "DELETE FROM embedding WHERE space_id=$1", sp.ID); err != nil {
			t.Fatal(err)
		}
		if err := EnsureVectorIndex(ctx, pool, "visual", sp); err != nil {
			t.Fatalf("repair invalid index: %v", err)
		}
		if ready, err := VectorIndexReady(ctx, pool, "visual", sp.ID); err != nil || !ready {
			t.Fatalf("repaired index ready=%v error=%v", ready, err)
		}
	})
}

func assertHNSWPlan(t *testing.T, ctx context.Context, pool *pgxpool.Pool, query VectorQuery) {
	t.Helper()
	sql, args, err := vectorSQL(query)
	if err != nil {
		t.Fatal(err)
	}
	err = db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL enable_seqscan=off; SET LOCAL hnsw.iterative_scan=relaxed_order"); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, "EXPLAIN "+sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		var plan string
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				return err
			}
			plan += line + "\n"
		}
		if !strings.Contains(plan, "Index Scan using "+indexName(query.Channel, query.Space.ID)) {
			t.Errorf("HNSW index not selected with sequential scan disabled:\n%s", plan)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
}

func indexTestDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("VH_TEST_DB_URL")
	if dsn == "" {
		t.Skip("set VH_TEST_DB_URL to videohelper_index_test for PostgreSQL integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqldb.Close() })
	var name string
	if err := sqldb.QueryRowContext(ctx, "SELECT current_database()").Scan(&name); err != nil || name != "videohelper_index_test" {
		t.Fatalf("refusing database %q: %v", name, err)
	}
	if err := migrations.Up(ctx, sqldb); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if err := migrations.Down(cleanupCtx, sqldb); err != nil {
			t.Error(err)
		}
	})
	pool, err := db.Open(ctx, dsn, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return ctx, pool
}

func insertWorkspace(t *testing.T, ctx context.Context, q db.Querier, name string) int64 {
	t.Helper()
	var id int64
	if err := q.QueryRow(ctx, "INSERT INTO workspace(name) VALUES($1) RETURNING id", name).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertVideo(t *testing.T, ctx context.Context, q db.Querier, workspaceID int64, status string, deleted bool) int64 {
	t.Helper()
	var id int64
	if err := q.QueryRow(ctx, "INSERT INTO video(workspace_id,title,source_kind,status,deleted_at) VALUES($1,'test','upload',$2,CASE WHEN $3 THEN now() END) RETURNING id", workspaceID, status, deleted).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func insertSpace(t *testing.T, ctx context.Context, q db.Querier, space VectorSpace) {
	t.Helper()
	if _, err := q.Exec(ctx, "INSERT INTO embedding_space(id,dim,metric,fingerprint,doc_inputs,query_inputs) VALUES($1,$2,$3,'test',ARRAY['video','text'],ARRAY['text'])", space.ID, space.Dim, space.Metric); err != nil {
		t.Fatal(err)
	}
}
