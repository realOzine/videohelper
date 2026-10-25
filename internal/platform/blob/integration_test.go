package blob

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestMinIOIntegration(t *testing.T) {
	endpoint := os.Getenv("VH_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set VH_TEST_S3_ENDPOINT to a disposable local MinIO endpoint")
	}
	t.Setenv("AWS_ACCESS_KEY_ID", "videohelper-local")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "videohelper-local-secret")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-1"))
	if err != nil {
		t.Fatal(err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true
	})
	bucket := fmt.Sprintf("videohelper-test-%d", time.Now().UnixNano())
	store := New(client, bucket)
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cleanupCancel()
		if err := store.DeletePrefix(cleanupCtx, "workspaces/"); err != nil {
			t.Error(err)
		}
		if _, err := client.DeleteBucket(cleanupCtx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)}); err != nil {
			t.Error(err)
		}
	})
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatalf("existing bucket: %v", err)
	}
	t.Run("presigned multipart", func(t *testing.T) {
		key := SourceKey(1, 1, "mp4")
		uploadID, err := store.CreateMultipart(ctx, key, "video/mp4")
		if err != nil {
			t.Fatal(err)
		}
		urls, err := store.PresignParts(ctx, key, uploadID, 2, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		first := bytes.Repeat([]byte("a"), 5*1024*1024)
		bodies := [][]byte{first, []byte("last part")}
		var parts []Part
		for i, part := range urls {
			req, err := http.NewRequestWithContext(ctx, http.MethodPut, part.URL, bytes.NewReader(bodies[i]))
			if err != nil {
				t.Fatal(err)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("upload part: %s", res.Status)
			}
			parts = append(parts, Part{N: part.N, ETag: res.Header.Get("ETag")})
		}
		parts[0], parts[1] = parts[1], parts[0]
		if err := store.CompleteMultipart(ctx, key, uploadID, parts); err != nil {
			t.Fatal(err)
		}
		if parts[0].N != 2 {
			t.Fatal("completion mutated caller's parts")
		}
		assertPresignedBody(t, ctx, store, key, append(first, bodies[1]...))
	})
	t.Run("stream and put", func(t *testing.T) {
		data := bytes.Repeat([]byte("s"), 17*1024*1024)
		key := SourceKey(1, 2, "mp4")
		n, err := store.PutStream(ctx, key, readOnly{bytes.NewReader(data)}, int64(len(data)))
		if err != nil || n != int64(len(data)) {
			t.Fatalf("stream n=%d err=%v", n, err)
		}
		assertPresignedBody(t, ctx, store, key, data)
		key = RawKey(1, 2, "asr-model/chunk-0.json")
		if err := store.Put(ctx, key, readOnly{strings.NewReader("{}")}, "application/json"); err != nil {
			t.Fatal(err)
		}
		assertPresignedBody(t, ctx, store, key, []byte("{}"))
		head, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		if err != nil || aws.ToString(head.ContentType) != "application/json" {
			t.Fatalf("Put content type = %v, error=%v", head, err)
		}
		if _, err := store.PutStream(ctx, RawKey(1, 2, "empty"), strings.NewReader(""), 0); err != nil {
			t.Fatalf("empty stream: %v", err)
		}
	})
	t.Run("failed streams are aborted", func(t *testing.T) {
		first := bytes.Repeat([]byte("f"), 16*1024*1024)
		readErr := errors.New("source read failed")
		for _, failure := range []string{"read", "truncated", "oversize", "cancel"} {
			t.Run(failure, func(t *testing.T) {
				uploadCtx, uploadCancel := context.WithCancel(ctx)
				defer uploadCancel()
				var tail io.Reader = errorReader{err: readErr}
				wantErr := readErr
				limit := int64(len(first) + 1)
				if failure == "truncated" {
					tail, wantErr = errorReader{err: io.ErrUnexpectedEOF}, io.ErrUnexpectedEOF
				} else if failure == "oversize" {
					tail, limit, wantErr = strings.NewReader("extra"), int64(len(first)), ErrTooLarge
				} else if failure == "cancel" {
					tail, wantErr = cancelReader{cancel: uploadCancel}, context.Canceled
				}
				key := RawKey(1, 3, failure)
				_, err := store.PutStream(uploadCtx, key, io.MultiReader(bytes.NewReader(first), tail), limit)
				if !errors.Is(err, wantErr) {
					t.Fatalf("error = %v, want %v", err, wantErr)
				}
				uploads, err := client.ListMultipartUploads(ctx, &s3.ListMultipartUploadsInput{Bucket: aws.String(bucket), Prefix: aws.String(key)})
				if err != nil || len(uploads.Uploads) != 0 {
					t.Fatalf("incomplete uploads remain: %v, error=%v", uploads, err)
				}
				if _, err := client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}); err == nil {
					t.Fatal("failed upload produced a completed object")
				}
			})
		}
	})
	t.Run("scoped delete", func(t *testing.T) {
		keep := SourceKey(2, 2, "mp4")
		if err := store.Put(ctx, keep, strings.NewReader("keep"), "video/mp4"); err != nil {
			t.Fatal(err)
		}
		if err := store.DeletePrefix(ctx, VideoPrefix(1, 2)); err != nil {
			t.Fatal(err)
		}
		listed, err := client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(VideoPrefix(1, 2))})
		if err != nil || len(listed.Contents) != 0 {
			t.Fatalf("objects remain after scoped delete: %v, error=%v", listed, err)
		}
		assertPresignedBody(t, ctx, store, keep, []byte("keep"))
		assertPresignedBody(t, ctx, store, SourceKey(1, 1, "mp4"), append(bytes.Repeat([]byte("a"), 5*1024*1024), []byte("last part")...))
	})
}

func assertPresignedBody(t *testing.T, ctx context.Context, store *Store, key string, want []byte) {
	t.Helper()
	url, err := store.PresignGet(ctx, key, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	got, err := io.ReadAll(res.Body)
	if err != nil || res.StatusCode != http.StatusOK || !bytes.Equal(got, want) {
		t.Fatalf("object %q: status=%s bytes=%d expected=%d error=%v", key, res.Status, len(got), len(want), err)
	}
}

type errorReader struct{ err error }

func (r errorReader) Read([]byte) (int, error) { return 0, r.err }

type cancelReader struct{ cancel context.CancelFunc }

func (r cancelReader) Read([]byte) (int, error) {
	r.cancel()
	return 0, context.Canceled
}
