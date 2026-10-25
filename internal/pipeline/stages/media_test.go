package stages

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/media"
	"github.com/ozine/videohelper/internal/pipeline"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/platform/queue"
	"github.com/ozine/videohelper/migrations"
)

func integration(t *testing.T) (context.Context, *pgxpool.Pool, *blob.Store) {
	t.Helper()
	dsn, endpoint := os.Getenv("VH_TEST_STAGES_DB_URL"), os.Getenv("VH_TEST_S3_ENDPOINT")
	if dsn == "" || endpoint == "" {
		t.Skip("set VH_TEST_STAGES_DB_URL and VH_TEST_S3_ENDPOINT to isolated local services")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	var count int
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename <> 'goose_db_version'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("refusing nonempty database: count=%d err=%v", count, err)
	}
	if err := migrations.Up(ctx, database); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := migrations.DownTo(context.Background(), database, 0); err != nil {
			t.Error(err)
		}
	})
	pool, err := db.Open(ctx, dsn, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	t.Setenv("AWS_ACCESS_KEY_ID", "videohelper-local")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "videohelper-local-secret")
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-1"))
	if err != nil {
		t.Fatal(err)
	}
	client := s3.NewFromConfig(cfg, func(options *s3.Options) { options.BaseEndpoint = aws.String(endpoint); options.UsePathStyle = true })
	bucket := "videohelper-stages-" + time.Now().Format("20060102150405.000000000")
	objects := blob.New(client, bucket)
	if err := objects.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := objects.DeletePrefix(context.Background(), "workspaces/"); err != nil {
			t.Error(err)
		}
		if _, err := client.DeleteBucket(context.Background(), &s3.DeleteBucketInput{Bucket: aws.String(bucket)}); err != nil {
			t.Error(err)
		}
	})
	return ctx, pool, objects
}

func commitRun(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sc *pipeline.StageCtx, run pipeline.RunFunc) {
	t.Helper()
	commit, err := run(ctx, sc)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := library.GetForWorker(ctx, tx, sc.Video.ID); err != nil {
			return err
		}
		return commit(ctx, tx)
	}); err != nil {
		t.Fatal(err)
	}
	sc.Video, err = library.GetForWorker(ctx, pool, sc.Video.ID)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMediaStagesIntegration(t *testing.T) {
	ctx, pool, objects := integration(t)
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg required")
	}
	directory := t.TempDir()
	file := filepath.Join(directory, "source.mp4")
	command := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=blue:s=128x72:r=10:d=2", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", file)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %s %v", output, err)
	}
	var workspaceID, videoID int64
	if err := pool.QueryRow(ctx, "INSERT INTO workspace(name) VALUES('stages') RETURNING id").Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO video(workspace_id,title,source_kind,status) VALUES($1,'fixture','upload','indexing') RETURNING id", workspaceID).Scan(&videoID); err != nil {
		t.Fatal(err)
	}
	h := New(pool, objects, nil, nil, Config{PresignTTL: time.Minute, Media: config.MediaConfig{NormalizedVideoMaxHeight: 72, NormalizedVideoGOPSeconds: 1, ThumbWidth: 32, SpriteCols: 2, SpriteRows: 2, TmpDir: directory}})
	source := blob.SourceKey(workspaceID, videoID, "mp4")
	if err := h.putFile(ctx, source, file, "video/mp4"); err != nil {
		t.Fatal(err)
	}
	if err := library.UpdateSource(ctx, pool, videoID, source); err != nil {
		t.Fatal(err)
	}
	video, err := library.GetForWorker(ctx, pool, videoID)
	if err != nil {
		t.Fatal(err)
	}
	sc := &pipeline.StageCtx{Video: video, Payload: queue.StagePayload{WorkspaceID: workspaceID, VideoID: videoID, Version: "v1.test"}, Attempt: 1, Def: pipeline.StageDef{Timeout: time.Minute}}
	commitRun(t, ctx, pool, sc, h.Probe)
	if sc.Video.DurationMs == nil || *sc.Video.DurationMs < 1900 || !*sc.Video.HasVideo || !*sc.Video.HasAudio {
		t.Fatalf("probe=%+v", sc.Video)
	}
	commitRun(t, ctx, pool, sc, h.PrepareMedia)
	if !strings.Contains(sc.Video.NormalizedVideoKey, "/runs/v1.test/1/") || sc.Video.Thumbs == nil || sc.Video.Thumbs.Count < 2 {
		t.Fatalf("prepared=%+v", sc.Video)
	}
	url, err := objects.PresignGet(ctx, sc.Video.NormalizedVideoKey, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	info, err := media.Probe(ctx, url)
	if err != nil || info.Height != 72 || !info.HasAudio {
		t.Fatalf("normalized=%+v err=%v", info, err)
	}
	url, err = objects.PresignGet(ctx, blob.SpriteKeyForVideo(sc.Video.NormalizedVideoKey, 0), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || len(body) == 0 || response.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("sprite status=%d bytes=%d err=%v", response.StatusCode, len(body), err)
	}
	commitRun(t, ctx, pool, sc, h.Finalize)
	var amount int
	if err := pool.QueryRow(ctx, "SELECT amount FROM usage_event WHERE workspace_id=$1 AND kind='indexed_ms'", workspaceID).Scan(&amount); err != nil || amount != *sc.Video.DurationMs || sc.Video.Status != "ready" {
		t.Fatalf("final status=%s amount=%d err=%v", sc.Video.Status, amount, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary preparation files remain: %v %v", entries, err)
	}
}
