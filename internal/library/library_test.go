package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/platform/authn"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/timeline"
	"github.com/ozine/videohelper/internal/workspace"
	"github.com/ozine/videohelper/migrations"
)

func integrationDB(t *testing.T) (context.Context, *pgxpool.Pool) {
	t.Helper()
	dsn := os.Getenv("VH_TEST_LIBRARY_DB_URL")
	if dsn == "" {
		t.Skip("set VH_TEST_LIBRARY_DB_URL to an empty disposable library database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqldb.Close() })
	var tables int
	countTables := "SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename <> 'goose_db_version'"
	if err := sqldb.QueryRowContext(ctx, countTables).Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("refusing nonempty database: tables=%d err=%v", tables, err)
	}
	if err := migrations.Up(ctx, sqldb); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		for {
			if err := sqldb.QueryRowContext(cleanup, countTables).Scan(&tables); err != nil {
				t.Error(err)
				return
			}
			if tables == 0 {
				return
			}
			if err := migrations.Down(cleanup, sqldb); err != nil {
				t.Error(err)
				return
			}
		}
	})
	pool, err := db.Open(ctx, dsn, 10)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return ctx, pool
}

func principal(t *testing.T, ctx context.Context, pool *pgxpool.Pool, subject string) workspace.Principal {
	t.Helper()
	p, err := workspace.New(pool).Resolve(ctx, authn.Claims{Sub: subject, Email: subject + "@example.test"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func fixtureVideo(t *testing.T, ctx context.Context, pool *pgxpool.Pool, p workspace.Principal, title string) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(ctx, "INSERT INTO video(workspace_id,title,source_kind,status,created_at) VALUES ($1,$2,'upload','indexing','2026-01-01') RETURNING id", p.WorkspaceID, title).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestLibraryMetadataIntegration(t *testing.T) {
	ctx, pool := integrationDB(t)
	s := New(pool, nil, Config{}, nil, nil)
	p, other := principal(t, ctx, pool, "local"), principal(t, ctx, pool, "foreign")
	a, b := fixtureVideo(t, ctx, pool, p, "甲"), fixtureVideo(t, ctx, pool, p, "乙")
	foreign := fixtureVideo(t, ctx, pool, other, "外部")
	deleted := fixtureVideo(t, ctx, pool, p, "已删除")
	if _, err := pool.Exec(ctx, "UPDATE video SET deleted_at=now() WHERE id=$1", deleted); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{foreign, deleted} {
		if _, err := s.Get(ctx, p, id); apperr.KindOf(err) != apperr.NotFound {
			t.Fatalf("hidden Get(%d): %v", id, err)
		}
	}
	first, next, err := s.List(ctx, p, ListFilter{Limit: 1})
	if err != nil || len(first) != 1 || first[0].ID != b || next == "" {
		t.Fatalf("first page: %v %q %v", first, next, err)
	}
	second, end, err := s.List(ctx, p, ListFilter{Limit: 1, Cursor: next})
	if err != nil || len(second) != 1 || second[0].ID != a || end != "" {
		t.Fatalf("second page: %v %q %v", second, end, err)
	}
	for _, filter := range []ListFilter{{Limit: 101}, {Cursor: "invalid"}, {Status: "deleted"}} {
		if _, _, err := s.List(ctx, p, filter); apperr.KindOf(err) != apperr.Invalid {
			t.Fatalf("invalid list filter: %v", err)
		}
	}
	project, err := s.CreateProject(ctx, p, " 学习 ")
	if err != nil || project.Name != "学习" {
		t.Fatalf("project: %+v %v", project, err)
	}
	foreignProject, err := s.CreateProject(ctx, other, "外部项目")
	if err != nil {
		t.Fatal(err)
	}
	if projects, err := s.ListProjects(ctx, p); err != nil || len(projects) != 1 || projects[0].ID != project.ID {
		t.Fatalf("project isolation: %v %v", projects, err)
	}
	if ids, err := s.ResolveScope(ctx, p, Scope{}); err != nil || ids != nil {
		t.Fatalf("all scope: %v %v", ids, err)
	}
	if ids, err := s.ResolveScope(ctx, p, Scope{VideoIDs: []int64{}}); err != nil || ids == nil || len(ids) != 0 {
		t.Fatalf("explicit empty scope: %v %v", ids, err)
	}
	if ids, err := s.ResolveScope(ctx, p, Scope{ProjectID: &project.ID}); err != nil || ids == nil || len(ids) != 0 {
		t.Fatalf("empty project scope: %v %v", ids, err)
	}
	for range 2 {
		if err := s.AddVideo(ctx, p, project.ID, a); err != nil {
			t.Fatal(err)
		}
	}
	for _, pair := range [][2]int64{{project.ID, foreign}, {foreignProject.ID, a}, {project.ID, deleted}} {
		if err := s.AddVideo(ctx, p, pair[0], pair[1]); apperr.KindOf(err) != apperr.NotFound {
			t.Fatalf("cross-workspace relation: %v", err)
		}
		if err := s.RemoveVideo(ctx, p, pair[0], pair[1]); apperr.KindOf(err) != apperr.NotFound {
			t.Fatalf("cross-workspace removal: %v", err)
		}
	}
	if ids, err := s.ResolveScope(ctx, p, Scope{ProjectID: &project.ID}); err != nil || !reflect.DeepEqual(ids, []int64{a}) {
		t.Fatalf("project scope: %v %v", ids, err)
	}
	if videos, _, err := s.List(ctx, p, ListFilter{ProjectID: &project.ID}); err != nil || len(videos) != 1 || videos[0].ID != a {
		t.Fatalf("project list: %v %v", videos, err)
	}
	for _, scope := range []Scope{{ProjectID: &foreignProject.ID}, {VideoIDs: []int64{a, foreign}}, {VideoIDs: []int64{deleted}}} {
		if _, err := s.ResolveScope(ctx, p, scope); apperr.KindOf(err) != apperr.NotFound {
			t.Fatalf("foreign/deleted scope: %v", err)
		}
	}
	if ids, err := s.ResolveScope(ctx, p, Scope{VideoIDs: []int64{b, a, b}}); err != nil || !reflect.DeepEqual(ids, []int64{a, b}) {
		t.Fatalf("explicit scope: %v %v", ids, err)
	}
	if err := s.DeleteProject(ctx, p, foreignProject.ID); apperr.KindOf(err) != apperr.NotFound {
		t.Fatalf("foreign project deletion: %v", err)
	}
	for range 2 {
		if err := s.RemoveVideo(ctx, p, project.ID, a); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddVideo(ctx, p, project.ID, a); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProject(ctx, p, project.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, p, a); err != nil {
		t.Fatalf("project deletion removed its video: %v", err)
	}

	t.Run("worker updates and row lock", func(t *testing.T) {
		if _, err := pool.Exec(ctx, "UPDATE video SET source_kind='direct', status='importing' WHERE id=$1", a); err != nil {
			t.Fatal(err)
		}
		layout := timeline.SpriteLayout{TileW: 320, TileH: 180, Cols: 10, Rows: 10, Count: 4}
		err := db.WithTx(ctx, pool, func(tx pgx.Tx) error {
			if _, err := GetForWorker(ctx, tx, a); err != nil {
				return err
			}
			if err := UpdateSource(ctx, tx, a, "workspaces/source.mp4"); err != nil {
				return err
			}
			if err := UpdateProbe(ctx, tx, a, 3500, true, false); err != nil {
				return err
			}
			if err := UpdatePreparedMedia(ctx, tx, a, "workspaces/normalized.mp4", layout); err != nil {
				return err
			}
			return SetStatus(ctx, tx, a, "ready", "")
		})
		if err != nil {
			t.Fatal(err)
		}
		video, err := s.Get(ctx, p, a)
		if err != nil || video.DurationMs == nil || *video.DurationMs != 3500 || video.HasAudio == nil || *video.HasAudio || video.Status != "ready" || video.Thumbs == nil || *video.Thumbs != layout {
			t.Fatalf("worker output: %+v %v", video, err)
		}
		var raw map[string]any
		data, _ := json.Marshal(video.Thumbs)
		json.Unmarshal(data, &raw)
		if raw["tile_w"] != float64(320) || len(raw) != 5 {
			t.Fatalf("sprite JSON contract: %s", data)
		}
		if durations, err := s.Durations(ctx, p.WorkspaceID, []int64{a, b, foreign, deleted}); err != nil || !reflect.DeepEqual(durations, map[int64]int{a: 3500}) {
			t.Fatalf("durations: %v %v", durations, err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := GetForWorker(ctx, tx, a); err != nil {
			t.Fatal(err)
		}
		waitCtx, stop := context.WithTimeout(ctx, 50*time.Millisecond)
		defer stop()
		if err := SetStatus(waitCtx, pool, a, "failed", "blocked"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("worker row was not locked: %v", err)
		}
	})
	for _, mutate := range []func() error{
		func() error { return UpdateSource(ctx, pool, deleted, "source.mp4") },
		func() error { return UpdateProbe(ctx, pool, deleted, 1000, true, true) },
		func() error { return SetStatus(ctx, pool, deleted, "ready", "") },
	} {
		if err := mutate(); apperr.KindOf(err) != apperr.NotFound {
			t.Fatalf("mutated deleted video: %v", err)
		}
	}
	if _, err := GetForWorker(ctx, pool, deleted); apperr.KindOf(err) != apperr.NotFound {
		t.Fatalf("worker saw deleted video: %v", err)
	}
}
