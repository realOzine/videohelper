package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func TestImportProjectDeleteRestorePurge(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	source := filepath.Join(t.TempDir(), "original.mp4")
	if err := os.WriteFile(source, []byte("user original"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "My project")
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.ImportFile(ctx, source, &p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "indexing" || v.SourceKey == "" {
		t.Fatalf("%+v", v)
	}
	list, err := s.ListVideos(ctx, VideoFilter{ProjectID: &p.ID})
	if err != nil || len(list) != 1 {
		t.Fatalf("%v %v", list, err)
	}
	if err := s.SetProjectVideos(ctx, p.ID, []int64{v.ID, 99999}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected rollback: %v", err)
	}
	list, err = s.ListVideos(ctx, VideoFilter{ProjectID: &p.ID})
	if err != nil || len(list) != 1 {
		t.Fatalf("project assignment did not roll back: %v %v", list, err)
	}
	if err := s.PurgeVideo(ctx, v.ID); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err := s.DeleteVideo(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	list, err = s.ListVideos(ctx, VideoFilter{})
	if err != nil || len(list) != 0 {
		t.Fatalf("%v %v", list, err)
	}
	if _, err := s.RestoreVideo(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteVideo(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.PurgeVideo(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Video(ctx, v.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != "user original" {
		t.Fatalf("original damaged: %q %v", got, err)
	}
}

func TestManagedFilesRejectEscapeAndCancel(t *testing.T) {
	s := testStore(t)
	for _, key := range []string{"../outside", "videos/1/../../outside", "/tmp/video", "videos/1/..\\outside", "videos/1/CON:stream"} {
		if _, err := s.OpenFile(key); err == nil {
			t.Fatalf("accepted %q", key)
		}
	}
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(s.dir, "media", "videos", "1"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(s.dir, "media", "videos", "1", "escape")); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteFile(context.Background(), "videos/1/escape/owned", nil); err == nil {
		t.Fatal("symlink escape accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.WriteFile(ctx, "videos/1/cancelled", nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := s.OpenFile("videos/1/cancelled"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestReopenPersists(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreateProject(ctx, "persistent")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ps, err := s.ListProjects(ctx)
	if err != nil || len(ps) != 1 || ps[0].ID != p.ID {
		t.Fatalf("%v %v", ps, err)
	}
}
