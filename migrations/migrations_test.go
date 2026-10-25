package migrations_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/migrations"
	"github.com/pgvector/pgvector-go"
)

// VH_TEST_DB_URL must identify an empty, disposable PostgreSQL database.
func TestMigrationAndDatabaseIntegration(t *testing.T) {
	dsn := os.Getenv("VH_TEST_DB_URL")
	if dsn == "" {
		t.Skip("set VH_TEST_DB_URL to an empty disposable database for integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqldb.Close() })
	var count int
	if err := sqldb.QueryRowContext(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename <> 'goose_db_version'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("integration database must have no application tables; refusing to migrate")
	}
	if err := migrations.Up(ctx, sqldb); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := migrations.DownTo(cleanupCtx, sqldb, 0); err != nil {
			t.Error(err)
		}
	})
	if err := migrations.Up(ctx, sqldb); err != nil {
		t.Fatalf("idempotent up: %v", err)
	}
	if err := sqldb.QueryRowContext(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename <> 'goose_db_version'").Scan(&count); err != nil || count != 12 {
		t.Fatalf("business table count = %d, error = %v", count, err)
	}
	pool, err := db.Open(ctx, dsn, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	input := pgvector.NewHalfVector([]float32{1, 0.5, -2})
	var output pgvector.HalfVector
	if err := pool.QueryRow(ctx, "SELECT $1::halfvec", input).Scan(&output); err != nil || !slices.Equal(input.Slice(), output.Slice()) {
		t.Fatalf("halfvec roundtrip = %v, error = %v", output, err)
	}
	sentinel := errors.New("rollback requested")
	err = db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "INSERT INTO workspace(name) VALUES ('rolled-back')"); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("transaction error = %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM workspace").Scan(&count); err != nil || count != 0 {
		t.Fatalf("rollback left %d rows, error = %v", count, err)
	}
	var workspaceID, videoID int64
	if err := db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, "INSERT INTO workspace(name) VALUES ('committed') RETURNING id").Scan(&workspaceID); err != nil {
			return err
		}
		return tx.QueryRow(ctx, "INSERT INTO video(workspace_id,title,source_kind,status) VALUES ($1,'video','upload','indexing') RETURNING id", workspaceID).Scan(&videoID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO text_span(video_id,workspace_id,kind,start_ms,end_ms,text,model) VALUES ($1,$2,'speech',0,100,'text','model')", videoID, workspaceID+1); err == nil {
		t.Fatal("cross-workspace text span was accepted")
	}
	if _, err := pool.Exec(ctx, "INSERT INTO text_span(video_id,workspace_id,kind,start_ms,end_ms,text,model) VALUES ($1,$2,'speech',-1,100,'text','model')", videoID, workspaceID); err == nil {
		t.Fatal("negative span start was accepted")
	}
	pool.Close()
	if err := migrations.DownTo(ctx, sqldb, 0); err != nil {
		t.Fatal(err)
	}
	if err := sqldb.QueryRowContext(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename <> 'goose_db_version'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("down left %d tables, error = %v", count, err)
	}
	if err := migrations.Up(ctx, sqldb); err != nil {
		t.Fatalf("reapply: %v", err)
	}
}
