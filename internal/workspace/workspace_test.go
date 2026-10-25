package workspace

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/platform/authn"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/migrations"
)

func TestPrincipalAndInvalidIdentity(t *testing.T) {
	ctx := context.Background()
	if _, ok := PrincipalFrom(ctx); ok {
		t.Fatal("unexpected identity")
	}
	want := Principal{UserID: 1, WorkspaceID: 2, Role: Owner}
	if got, ok := PrincipalFrom(WithPrincipal(ctx, want)); !ok || got != want {
		t.Fatalf("principal = %+v, %v", got, ok)
	}
	if _, err := New(nil).Resolve(ctx, authn.Claims{}, 0); apperr.KindOf(err) != apperr.Unauthenticated {
		t.Fatalf("empty subject = %v", err)
	}
	if _, err := New(nil).Resolve(ctx, authn.Claims{Sub: "user"}, -1); apperr.KindOf(err) != apperr.Invalid {
		t.Fatalf("negative workspace = %v", err)
	}
}

func TestResolveIntegration(t *testing.T) {
	dsn := os.Getenv("VH_TEST_WORKSPACE_DB_URL")
	if dsn == "" {
		t.Skip("set VH_TEST_WORKSPACE_DB_URL to an empty disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sqldb.Close()
	var tables int
	if err := sqldb.QueryRowContext(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename <> 'goose_db_version'").Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("refusing nonempty database: tables=%d err=%v", tables, err)
	}
	if err := migrations.Up(ctx, sqldb); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := migrations.Down(ctx, sqldb); err != nil {
			t.Error(err)
		}
	}()
	pool, err := db.Open(ctx, dsn, 10)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	service := New(pool)
	claims := authn.Claims{Sub: "same-user", Email: "person@example.test", Name: "Person"}
	var wg sync.WaitGroup
	results := make(chan Principal, 10)
	for range 10 {
		wg.Go(func() {
			p, err := service.Resolve(ctx, claims, 0)
			if err != nil {
				t.Error(err)
				return
			}
			results <- p
		})
	}
	wg.Wait()
	close(results)
	var first Principal
	for p := range results {
		if first == (Principal{}) {
			first = p
		}
		if first != p || p.Role != Owner {
			t.Fatalf("duplicate provision: first=%+v current=%+v", first, p)
		}
	}
	if first.UserID == 0 || first.WorkspaceID == 0 {
		t.Fatal("no identity resolved")
	}
	var users, workspaces, memberships int
	if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM app_user),(SELECT count(*) FROM workspace),(SELECT count(*) FROM membership)").Scan(&users, &workspaces, &memberships); err != nil || users != 1 || workspaces != 1 || memberships != 1 {
		t.Fatalf("provision counts=%d,%d,%d err=%v", users, workspaces, memberships, err)
	}
	other, err := service.Resolve(ctx, authn.Claims{Sub: "other-user"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if p, err := service.Resolve(ctx, claims, other.WorkspaceID); apperr.KindOf(err) != apperr.Forbidden || p != (Principal{}) {
		t.Fatalf("foreign workspace returned %+v err=%v", p, err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO membership(workspace_id,user_id,role) VALUES ($1,$2,'member')", other.WorkspaceID, first.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Resolve(ctx, claims, 0); apperr.KindOf(err) != apperr.Invalid {
		t.Fatalf("ambiguous membership: %v", err)
	}
	if p, err := service.Resolve(ctx, claims, other.WorkspaceID); err != nil || p.Role != Member {
		t.Fatalf("explicit membership: %+v %v", p, err)
	}
	if _, err := service.Resolve(ctx, authn.Claims{Sub: "new-user-with-invalid-workspace"}, other.WorkspaceID); apperr.KindOf(err) != apperr.Forbidden {
		t.Fatalf("foreign workspace on provisioning: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM app_user WHERE auth_sub='new-user-with-invalid-workspace'").Scan(&users); err != nil || users != 0 {
		t.Fatalf("failed selection must roll back provisioning, users=%d err=%v", users, err)
	}
}
