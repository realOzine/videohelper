package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func fixtureVideo(t *testing.T, s *Store) Video {
	t.Helper()
	source := filepath.Join(t.TempDir(), "sample.mp4")
	if err := os.WriteFile(source, []byte("store test media bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	v, err := s.ImportFile(context.Background(), source, nil)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestJobCheckpointRollbackAndFencing(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	v := fixtureVideo(t, s)
	j, err := s.CreateJob(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	j2, err := s.CreateJob(ctx, v.ID)
	if err != nil || j2.ID != j.ID {
		t.Fatalf("not idempotent: %v %v", j2, err)
	}
	j, err = s.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("rollback output")
	err = s.Checkpoint(ctx, j.ID, j.Attempt, "probe", map[string]int{"next": 1}, func(tx *Tx) error {
		if err := tx.UpdateMedia(ctx, v.ID, MediaInfo{DurationMs: 6000, HasVideo: true}); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatal(err)
	}
	after, err := s.Video(ctx, v.ID)
	if err != nil || after.DurationMs != nil {
		t.Fatalf("output leaked: %+v %v", after, err)
	}
	afterJob, err := s.Job(ctx, j.ID)
	if err != nil || string(afterJob.Progress) != "{}" {
		t.Fatalf("checkpoint leaked: %+v %v", afterJob, err)
	}
	if err := s.CancelJob(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJob(ctx, j.ID, j.Attempt, nil); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if _, err := s.RetryJob(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	newAttempt, err := s.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if newAttempt.Attempt <= j.Attempt {
		t.Fatalf("attempt did not advance: %+v %+v", j, newAttempt)
	}
	if err := s.Checkpoint(ctx, j.ID, j.Attempt, "probe", nil, nil); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	if err := s.CompleteJob(ctx, newAttempt.ID, newAttempt.Attempt, func(tx *Tx) error { return tx.UpdateMedia(ctx, v.ID, MediaInfo{DurationMs: 6000, HasVideo: true}) }); err != nil {
		t.Fatal(err)
	}
	after, err = s.Video(ctx, v.ID)
	if err != nil || after.Status != "ready" || after.DurationMs == nil {
		t.Fatalf("%+v %v", after, err)
	}
}

func TestJobRecoveryAndImportCrashGap(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	v := fixtureVideo(t, s)
	urlVideo, err := s.CreateImport(ctx, "https://www.youtube.com/watch?v=example", "youtube", "import", nil)
	if err != nil {
		t.Fatal(err)
	}
	count, err := s.RecoverJobs(ctx)
	if err != nil || count != 2 {
		t.Fatalf("gap recovery %d %v", count, err)
	}
	j, err := s.LatestJob(ctx, urlVideo.ID)
	if err != nil || j.Stage != "fetch" {
		t.Fatalf("%+v %v", j, err)
	}
	claimed, err := s.ClaimJob(ctx)
	if err != nil || claimed.VideoID != v.ID {
		t.Fatalf("%+v %v", claimed, err)
	}
	if err := s.Checkpoint(ctx, claimed.ID, claimed.Attempt, "asr", map[string]int{"next": 2}, nil); err != nil {
		t.Fatal(err)
	}
	count, err = s.RecoverJobs(ctx)
	if err != nil || count != 1 {
		t.Fatalf("running recovery %d %v", count, err)
	}
	if err := s.CompleteJob(ctx, claimed.ID, claimed.Attempt, nil); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	newClaim, err := s.ClaimJob(ctx)
	if err != nil || newClaim.Stage != "asr" || string(newClaim.Progress) != "{\"next\":2}" {
		t.Fatalf("%+v %v", newClaim, err)
	}
	if err := s.CancelJob(ctx, newClaim.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecoverJobs(ctx); err != nil {
		t.Fatal(err)
	}
	jobs, err := s.ListJobs(ctx, v.ID)
	if err != nil || len(jobs) != 1 || jobs[0].Status != "cancelled" {
		t.Fatalf("cancel auto restarted: %v %v", jobs, err)
	}
}

func TestConcurrentClaimAndDeleteFence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	v := fixtureVideo(t, s)
	if _, err := s.CreateJob(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	var claimed atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := s.ClaimJob(ctx)
			if err == nil {
				claimed.Add(1)
			} else if !errors.Is(err, ErrNotFound) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if claimed.Load() != 1 {
		t.Fatal(claimed.Load())
	}
	j, err := s.LatestJob(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteVideo(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJob(ctx, j.ID, j.Attempt, nil); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
}

func TestDeleteRestoreInterruptedVideoCanRetryWithoutProcessor(t *testing.T) {
	for _, running := range []bool{false, true} {
		name := "queued_import"
		if running {
			name = "running_index"
		}
		t.Run(name, func(t *testing.T) {
			s := testStore(t)
			ctx := context.Background()
			var v Video
			var err error
			if running {
				v = fixtureVideo(t, s)
			} else {
				v, err = s.CreateImport(ctx, "https://example.com/video.mp4", "direct", "queued download", nil)
				if err != nil {
					t.Fatal(err)
				}
			}
			j, err := s.CreateJob(ctx, v.ID)
			if err != nil {
				t.Fatal(err)
			}
			if running {
				j, err = s.ClaimJob(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if err := s.Checkpoint(ctx, j.ID, j.Attempt, "asr", map[string]int{"next": 2}, nil); err != nil {
					t.Fatal(err)
				}
			}
			before, err := s.Job(ctx, j.ID)
			if err != nil {
				t.Fatal(err)
			}
			other := fixtureVideo(t, s)
			otherJob, err := s.CreateJob(ctx, other.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.DeleteVideo(ctx, v.ID); err != nil {
				t.Fatal(err)
			}
			if err := s.CompleteJob(ctx, j.ID, j.Attempt, nil); !errors.Is(err, ErrStale) {
				t.Fatal(err)
			}
			restored, err := s.RestoreVideo(ctx, v.ID)
			if err != nil || restored.Status != "failed" || restored.Error != "任务已取消" || restored.SourceKey != v.SourceKey {
				t.Fatalf("restored video is stuck: %+v %v", restored, err)
			}
			if n, err := s.RecoverJobs(ctx); err != nil || n != 0 {
				t.Fatalf("cancelled job was auto-restarted: %d %v", n, err)
			}
			cancelled, err := s.Job(ctx, j.ID)
			if err != nil || cancelled.Status != "cancelled" || string(cancelled.Progress) != string(before.Progress) {
				t.Fatalf("checkpoint changed: %+v %v", cancelled, err)
			}
			unaffected, err := s.Job(ctx, otherJob.ID)
			if err != nil || unaffected.Status != "queued" {
				t.Fatalf("another video was cancelled: %+v %v", unaffected, err)
			}
			if _, err := s.RetryJob(ctx, j.ID); err != nil {
				t.Fatal(err)
			}
			claimed, err := s.ClaimJob(ctx)
			if err != nil || claimed.ID != j.ID || claimed.Attempt <= j.Attempt {
				t.Fatalf("retry did not claim a fresh attempt: %+v %v", claimed, err)
			}
			if v.SourceKey != "" {
				f, err := s.OpenFile(v.SourceKey)
				if err != nil {
					t.Fatal(err)
				}
				f.Close()
			}
		})
	}
}
