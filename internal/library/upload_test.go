package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/timeline"
)

type testStarter struct {
	pool  *pgxpool.Pool
	calls atomic.Int32
	fail  atomic.Bool
}

var errTestStart = errors.New("explicit test starter failed")

func (s *testStarter) Start(ctx context.Context, id int64) error {
	s.calls.Add(1)
	if s.fail.Load() {
		return errTestStart
	}
	return db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := GetForWorker(ctx, tx, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "INSERT INTO stage_run(video_id,stage,version,status) VALUES ($1,'probe','test','pending') ON CONFLICT DO NOTHING", id); err != nil {
			return err
		}
		return SetStatus(ctx, tx, id, "indexing", "")
	})
}

type testDropper struct{ fail atomic.Bool }

func (d *testDropper) DropDerived(ctx context.Context, q db.Querier, id int64) error {
	for _, query := range []string{"DELETE FROM embedding WHERE video_id=$1", "DELETE FROM text_span WHERE video_id=$1", "DELETE FROM stage_run WHERE video_id=$1"} {
		if _, err := q.Exec(ctx, query, id); err != nil {
			return err
		}
	}
	if d.fail.Load() {
		return errors.New("explicit test dropper failed")
	}
	return nil
}

func integrationObjects(t *testing.T, ctx context.Context) (*blob.Store, *s3.Client, string) {
	t.Helper()
	endpoint := os.Getenv("VH_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set VH_TEST_S3_ENDPOINT to a disposable local MinIO endpoint")
	}
	t.Setenv("AWS_ACCESS_KEY_ID", "videohelper-local")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "videohelper-local-secret")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-1"))
	if err != nil {
		t.Fatal(err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) { o.BaseEndpoint = aws.String(endpoint); o.UsePathStyle = true })
	bucket := fmt.Sprintf("videohelper-library-%d", time.Now().UnixNano())
	store := blob.New(client, bucket)
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		uploads, err := client.ListMultipartUploads(cleanup, &s3.ListMultipartUploadsInput{Bucket: aws.String(bucket)})
		if err != nil {
			t.Error(err)
		} else {
			for _, upload := range uploads.Uploads {
				if err := store.AbortMultipart(cleanup, aws.ToString(upload.Key), aws.ToString(upload.UploadId)); err != nil {
					t.Error(err)
				}
			}
		}
		if err := store.DeletePrefix(cleanup, "workspaces/"); err != nil {
			t.Error(err)
		}
		if _, err := client.DeleteBucket(cleanup, &s3.DeleteBucketInput{Bucket: aws.String(bucket)}); err != nil {
			t.Error(err)
		}
	})
	return store, client, bucket
}

func uploadParts(t *testing.T, ctx context.Context, plan UploadPlan, data []byte) []blob.Part {
	t.Helper()
	parts := make([]blob.Part, 0, len(plan.Parts))
	for _, part := range plan.Parts {
		start := int64(part.N-1) * plan.PartSize
		end := min(start+plan.PartSize, int64(len(data)))
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, part.URL, bytes.NewReader(data[start:end]))
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("PUT part: %s", response.Status)
		}
		parts = append(parts, blob.Part{N: part.N, ETag: response.Header.Get("ETag")})
	}
	return parts
}

func TestUploadSizing(t *testing.T) {
	for _, size := range []int64{1, 5 * mebibyte, 5*mebibyte + 1, maxObjectBytes} {
		part, count, err := uploadSizing(size, 5*mebibyte)
		if err != nil || count < 1 || count > 10000 || part < 5*mebibyte || int64(count)*part < size {
			t.Fatalf("size %d => %d/%d, %v", size, part, count, err)
		}
	}
	for _, size := range []int64{-1, 0, maxObjectBytes + 1} {
		if _, _, err := uploadSizing(size, 5*mebibyte); err == nil {
			t.Fatalf("accepted size %d", size)
		}
	}
}

func TestUploadLifecycleIntegration(t *testing.T) {
	if os.Getenv("VH_TEST_S3_ENDPOINT") == "" {
		t.Skip("set VH_TEST_S3_ENDPOINT for real MinIO library integration")
	}
	ctx, pool := integrationDB(t)
	objects, s3client, bucket := integrationObjects(t, ctx)
	starter, dropper := &testStarter{pool: pool}, &testDropper{}
	s := New(pool, objects, Config{UploadPartBytes: 5 * mebibyte}, starter, dropper)
	p, foreign := principal(t, ctx, pool, "upload-owner"), principal(t, ctx, pool, "upload-foreign")
	project, err := s.CreateProject(ctx, p, "上传项目")
	if err != nil {
		t.Fatal(err)
	}
	data := bytes.Repeat([]byte("v"), int(5*mebibyte+1))
	video, plan, err := s.StartUpload(ctx, p, "folder/演示.MP4", int64(len(data)), "video/mp4", &project.ID)
	if err != nil || video.Title != "演示.MP4" || video.SourceKey != "" || video.Status != "uploading" || len(plan.Parts) != 2 {
		t.Fatalf("StartUpload: %+v %+v %v", video, plan, err)
	}
	parts := uploadParts(t, ctx, plan, data)
	if _, err := s.CompleteUpload(ctx, foreign, video.ID, plan.UploadID, parts); apperr.KindOf(err) != apperr.NotFound {
		t.Fatalf("foreign completion: %v", err)
	}
	if _, err := s.CompleteUpload(ctx, p, video.ID, "another-upload", parts); apperr.KindOf(err) != apperr.Invalid {
		t.Fatalf("unbound upload ID: %v", err)
	}
	if _, err := s.CompleteUpload(ctx, p, video.ID, plan.UploadID, parts[:1]); apperr.KindOf(err) != apperr.Invalid {
		t.Fatalf("missing part accepted: %v", err)
	}
	starter.fail.Store(true)
	if _, err := s.CompleteUpload(ctx, p, video.ID, plan.UploadID, parts); !errors.Is(err, errTestStart) {
		t.Fatalf("Start failure: %v", err)
	}
	known, err := s.Get(ctx, p, video.ID)
	if err != nil || known.SourceKey == "" || known.Status != "uploading" {
		t.Fatalf("unrecoverable completion: %+v %v", known, err)
	}
	if info, err := objects.Head(ctx, known.SourceKey); err != nil || info.Size != int64(len(data)) {
		t.Fatalf("completed object: %+v %v", info, err)
	}
	starter.fail.Store(false)
	video, err = s.CompleteUpload(ctx, p, video.ID, plan.UploadID, parts)
	if err != nil || video.Status != "indexing" {
		t.Fatalf("completion retry: %+v %v", video, err)
	}
	calls := starter.calls.Load()
	if _, err := s.CompleteUpload(ctx, p, video.ID, plan.UploadID, parts); err != nil || starter.calls.Load() != calls {
		t.Fatalf("duplicate completion restarted indexing: %v", err)
	}
	if _, _, err := s.PlaybackURL(ctx, p, video.ID); apperr.KindOf(err) != apperr.Conflict {
		t.Fatalf("unprepared playback: %v", err)
	}
	normalized := blob.NormalizedVideoKey(p.WorkspaceID, video.ID)
	if err := objects.Put(ctx, normalized, strings.NewReader("explicit storage fixture"), "video/mp4"); err != nil {
		t.Fatal(err)
	}
	if err := UpdatePreparedMedia(ctx, pool, video.ID, normalized, timeline.SpriteLayout{TileW: 320, TileH: 180, Cols: 10, Rows: 10, Count: 1}); err != nil {
		t.Fatal(err)
	}
	url, expiry, err := s.PlaybackURL(ctx, p, video.ID)
	if err != nil || expiry.Before(time.Now()) {
		t.Fatalf("PlaybackURL: %v %v", expiry, err)
	}
	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != "explicit storage fixture" {
		t.Fatalf("signed playback body: %s %q", response.Status, body)
	}
	if _, _, err := s.PlaybackURL(ctx, foreign, video.ID); apperr.KindOf(err) != apperr.NotFound {
		t.Fatalf("foreign playback: %v", err)
	}

	t.Run("delete rollback and worker race", func(t *testing.T) {
		dropper.fail.Store(true)
		if err := s.Delete(ctx, p, video.ID); err == nil {
			t.Fatal("ignored derived-drop failure")
		}
		if _, err := s.Get(ctx, p, video.ID); err != nil {
			t.Fatalf("failed delete did not roll back: %v", err)
		}
		var stages int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM stage_run WHERE video_id=$1", video.ID).Scan(&stages); err != nil || stages != 1 {
			t.Fatalf("drop rollback: stages=%d %v", stages, err)
		}
		dropper.fail.Store(false)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err := GetForWorker(ctx, tx, video.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO text_span(video_id,workspace_id,kind,start_ms,end_ms,text,model) VALUES ($1,$2,'speech',0,1000,'测试','test-asr')", video.ID, p.WorkspaceID); err != nil {
			t.Fatal(err)
		}
		result := make(chan error, 1)
		go func() { result <- s.Delete(ctx, p, video.ID) }()
		select {
		case err := <-result:
			t.Fatalf("delete ignored worker lock: %v", err)
		case <-time.After(50 * time.Millisecond):
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-result:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("delete remained blocked after worker commit")
		}
		if _, err := s.Get(ctx, p, video.ID); apperr.KindOf(err) != apperr.NotFound {
			t.Fatalf("deleted video visible: %v", err)
		}
		if _, _, err := s.PlaybackURL(ctx, p, video.ID); apperr.KindOf(err) != apperr.NotFound {
			t.Fatalf("deleted playback visible: %v", err)
		}
		if _, err := GetForWorker(ctx, pool, video.ID); apperr.KindOf(err) != apperr.NotFound {
			t.Fatalf("worker resurrected deleted video: %v", err)
		}
		if err := pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM stage_run WHERE video_id=$1)+(SELECT count(*) FROM text_span WHERE video_id=$1)+(SELECT count(*) FROM embedding WHERE video_id=$1)", video.ID).Scan(&stages); err != nil || stages != 0 {
			t.Fatalf("derived rows survived delete: %d %v", stages, err)
		}
		if _, err := objects.Head(ctx, known.SourceKey); err != nil {
			t.Fatalf("soft delete removed source: %v", err)
		}
		if err := s.Delete(ctx, p, video.ID); err != nil {
			t.Fatalf("repeat soft delete: %v", err)
		}
		starter.fail.Store(true)
		if _, err := s.Restore(ctx, p, video.ID); !errors.Is(err, errTestStart) {
			t.Fatalf("restore Start failure: %v", err)
		}
		restored, err := s.Get(ctx, p, video.ID)
		if err != nil || restored.Status != "uploading" || restored.SourceKey != known.SourceKey {
			t.Fatalf("restore not R4-recoverable: %+v %v", restored, err)
		}
		starter.fail.Store(false)
		if err := starter.Start(ctx, video.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Restore(ctx, p, video.ID); apperr.KindOf(err) != apperr.Conflict {
			t.Fatalf("restored active video twice: %v", err)
		}
		if ids, err := s.ResolveScope(ctx, p, Scope{ProjectID: &project.ID}); err != nil || len(ids) != 1 || ids[0] != video.ID {
			t.Fatalf("project association lost: %v %v", ids, err)
		}
	})

	t.Run("declared size mismatch", func(t *testing.T) {
		v, plan, err := s.StartUpload(ctx, p, "mismatch.mp4", 10, "video/mp4", nil)
		if err != nil {
			t.Fatal(err)
		}
		parts := uploadParts(t, ctx, plan, []byte("bad"))
		if _, err := s.CompleteUpload(ctx, p, v.ID, plan.UploadID, parts); apperr.KindOf(err) != apperr.Invalid {
			t.Fatalf("wrong object size accepted: %v", err)
		}
		v, err = s.Get(ctx, p, v.ID)
		if err != nil || v.SourceKey != "" {
			t.Fatalf("wrong size recorded source: %+v %v", v, err)
		}
	})

	t.Run("unfinished upload restores its trusted session", func(t *testing.T) {
		v, plan, err := s.StartUpload(ctx, p, "unfinished.mp4", 4, "video/mp4", nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Delete(ctx, p, v.ID); err != nil {
			t.Fatal(err)
		}
		before := starter.calls.Load()
		restored, err := s.Restore(ctx, p, v.ID)
		if err != nil || restored.Status != "uploading" || restored.SourceKey != "" || starter.calls.Load() != before {
			t.Fatalf("unfinished restore: %+v %v", restored, err)
		}
		parts := uploadParts(t, ctx, plan, []byte("test"))
		if _, err := s.CompleteUpload(ctx, p, v.ID, plan.UploadID, parts); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("session persistence failure aborts multipart", func(t *testing.T) {
		if _, err := pool.Exec(ctx, `CREATE FUNCTION reject_upload_session() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN RAISE EXCEPTION ''test session rejection''; END'; CREATE TRIGGER reject_upload_session BEFORE INSERT ON video_upload FOR EACH ROW EXECUTE FUNCTION reject_upload_session()`); err != nil {
			t.Fatal(err)
		}
		defer pool.Exec(ctx, "DROP TRIGGER reject_upload_session ON video_upload; DROP FUNCTION reject_upload_session()")
		if _, _, err := s.StartUpload(ctx, p, "rejected.mp4", 10, "video/mp4", nil); err == nil {
			t.Fatal("ignored session persistence failure")
		}
		var rows int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM video WHERE title='rejected.mp4'").Scan(&rows); err != nil || rows != 0 {
			t.Fatalf("failed registration left video: %d %v", rows, err)
		}
		uploads, err := s3client.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{Bucket: aws.String(bucket)})
		if err != nil || len(uploads.Uploads) != 0 {
			t.Fatalf("failed registration leaked multipart session: %v %v", uploads, err)
		}
	})
}
