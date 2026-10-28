package store

import (
	"context"
	"errors"
	"testing"
)

func TestCancelledVideoDerivedStatusAndLifecycle(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p, err := s.CreateProject(ctx, "personal")
	if err != nil {
		t.Fatal(err)
	}
	v := fixtureVideo(t, s)
	if err := s.AddProjectVideo(ctx, p.ID, v.ID); err != nil {
		t.Fatal(err)
	}
	j, err := s.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CancelJob(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	assertStatus := func(want string) {
		t.Helper()
		got, err := s.Video(ctx, v.ID)
		if err != nil || got.Status != want {
			t.Fatalf("video=%+v err=%v want=%s", got, err, want)
		}
	}
	assertStatus("cancelled")
	// Existing v1 databases already store failed + cancelled. No schema/data
	// rewrite is required, and failure filtering uses the same derived state.
	var stored string
	var version int
	if err := s.db.QueryRowContext(ctx, "SELECT status FROM video WHERE id=?", v.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if stored != "failed" || version != 1 {
		t.Fatalf("stored=%s version=%d", stored, version)
	}
	for _, f := range []VideoFilter{{Status: "failed"}, {Status: "cancelled", ProjectID: &p.ID}, {Status: "cancelled", IDs: []int64{v.ID}}} {
		rows, err := s.ListVideos(ctx, f)
		want := 1
		if f.Status == "failed" {
			want = 0
		}
		if err != nil || len(rows) != want {
			t.Fatalf("filter=%+v rows=%+v err=%v", f, rows, err)
		}
	}
	before := v.ID
	if rows, err := s.ListVideos(ctx, VideoFilter{Status: "cancelled", BeforeID: &before}); err != nil || len(rows) != 0 {
		t.Fatalf("cursor=%+v %v", rows, err)
	}
	if n, err := s.RecoverJobs(ctx); err != nil || n != 0 {
		t.Fatalf("recovered cancellation: %d %v", n, err)
	}
	if err := s.CompleteJob(ctx, j.ID, j.Attempt, nil); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if err := s.DeleteVideo(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	if rows, err := s.ListVideos(ctx, VideoFilter{Status: "cancelled"}); err != nil || len(rows) != 0 {
		t.Fatalf("deleted cancellation=%+v %v", rows, err)
	}
	if _, err := s.RestoreVideo(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	assertStatus("cancelled")
	if _, err := s.RetryJob(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	assertStatus("indexing")
	j, err = s.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.RecoverJobs(ctx); err != nil || n != 1 {
		t.Fatalf("retry recovery=%d %v", n, err)
	}
	j, err = s.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FailJob(ctx, j.ID, j.Attempt, "damaged media"); err != nil {
		t.Fatal(err)
	}
	assertStatus("failed")
	if _, err := s.RetryJob(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	j, err = s.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJob(ctx, j.ID, j.Attempt, nil); err != nil {
		t.Fatal(err)
	}
	assertStatus("ready")
	// A newer job wins over retained cancelled history.
	j, err = s.CreateJob(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CancelJob(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateJob(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	j, err = s.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FailJob(ctx, j.ID, j.Attempt, "new failure"); err != nil {
		t.Fatal(err)
	}
	assertStatus("failed")
}
