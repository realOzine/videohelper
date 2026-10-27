package store

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestFailedImportsRemoveOnlyUnsubmittedData(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	accepted := fixtureVideo(t, s)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.ImportFile(cancelled, "cancelled.mp4", nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := s.ImportFile(ctx, filepath.Join(t.TempDir(), "missing.mp4"), nil); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "source.mp4")
	if err := os.WriteFile(source, []byte("user original"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TRIGGER reject_job BEFORE INSERT ON job BEGIN SELECT RAISE(ABORT,'queue failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportFile(ctx, source, nil); err == nil {
		t.Fatal("source committed without its durable job")
	}
	if _, err := s.ImportURL(ctx, "https://example.com/video.mp4", nil); err == nil {
		t.Fatal("URL committed without its durable job")
	}
	rows, err := s.ListVideos(ctx, VideoFilter{})
	if err != nil || len(rows) != 1 || rows[0].ID != accepted.ID {
		t.Fatalf("failed imports left library records: %+v %v", rows, err)
	}
	entries, err := fs.ReadDir(s.root.FS(), "videos")
	if err != nil || len(entries) != 1 || entries[0].Name() != strconv.FormatInt(accepted.ID, 10) {
		t.Fatalf("failed imports left managed media: %+v %v", entries, err)
	}
	if j, err := s.LatestJob(ctx, accepted.ID); err != nil || j.Status != "queued" {
		t.Fatalf("prior job was damaged: %+v %v", j, err)
	}
	if data, err := os.ReadFile(source); err != nil || string(data) != "user original" {
		t.Fatalf("user source changed: %q %v", data, err)
	}
}

func TestCompleteFileCopyCommitsSourceAndJobAfterCallerCancel(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	var id int64
	if err := s.write(ctx, func(tx *sql.Tx) error {
		var err error
		id, err = insertVideo(ctx, tx, "upload", "", "source", "source.mp4", nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "source.mp4")
	if err := os.WriteFile(source, []byte("complete managed copy"), 0600); err != nil {
		t.Fatal(err)
	}
	key, err := s.CopyFile(ctx, id, source)
	if err != nil {
		t.Fatal(err)
	}
	caller, cancel := context.WithCancel(ctx)
	cancel()
	v, err := s.commitFileImport(caller, id, key)
	if err != nil || v.SourceKey != key || v.Status != "indexing" {
		t.Fatalf("accepted copy lost its handoff: %+v %v", v, err)
	}
	j, err := s.LatestJob(ctx, id)
	if err != nil || j.Status != "queued" || j.Stage != "probe" {
		t.Fatalf("source has no durable job: %+v %v", j, err)
	}
	if duplicate, err := s.CreateJob(ctx, id); err != nil || duplicate.ID != j.ID {
		t.Fatalf("handoff duplicated a job: %+v %v", duplicate, err)
	}
}

func TestImportCleanupPreservesAnyAcceptedSourceOrJob(t *testing.T) {
	for _, state := range []string{"source_and_job", "source_only", "job_only"} {
		t.Run(state, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			v := fixtureVideo(t, s)
			if state == "source_only" {
				if _, err := s.db.ExecContext(ctx, "DELETE FROM job WHERE video_id=?", v.ID); err != nil {
					t.Fatal(err)
				}
			}
			if state == "job_only" {
				if _, err := s.db.ExecContext(ctx, "UPDATE video SET source_key='' WHERE id=?", v.ID); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.discardImport(ctx, v.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.Video(ctx, v.ID); err != nil {
				t.Fatal("accepted video removed", err)
			}
			f, err := s.OpenFile(v.SourceKey)
			if err != nil {
				t.Fatal("accepted media removed", err)
			}
			f.Close()
			if state != "source_only" {
				if _, err := s.LatestJob(ctx, v.ID); err != nil {
					t.Fatal("accepted job removed", err)
				}
			}
		})
	}
}
