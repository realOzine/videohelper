package blob

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestKeys(t *testing.T) {
	prefix := "workspaces/7/videos/42/"
	for got, want := range map[string]string{
		VideoPrefix(7, 42):                      prefix,
		SourceKey(7, 42, ".mp4"):                prefix + "source.mp4",
		NormalizedVideoKey(7, 42):               prefix + "normalized_video.mp4",
		SpriteKey(7, 42, 2):                     prefix + "sprites/2.jpg",
		RawKey(7, 42, "asr-model/chunk-0.json"): prefix + "raw/asr-model/chunk-0.json",
	} {
		if got != want {
			t.Fatalf("key = %q, want %q", got, want)
		}
	}
	if got := (&Store{bucket: "videohelper"}).URI(NormalizedVideoKey(7, 42)); got != "s3://videohelper/"+prefix+"normalized_video.mp4" {
		t.Fatalf("URI = %q", got)
	}
}

func TestInvalidInputsFailBeforeS3(t *testing.T) {
	store := &Store{}
	ctx := context.Background()
	for _, count := range []int{0, maxParts + 1} {
		if _, err := store.PresignParts(ctx, "key", "upload", count, time.Hour); err == nil {
			t.Fatal("invalid part count accepted")
		}
	}
	for _, parts := range [][]Part{nil, {{N: 0, ETag: "tag"}}, {{N: 1}}, {{N: 1, ETag: "a"}, {N: 1, ETag: "b"}}} {
		if err := store.CompleteMultipart(ctx, "key", "upload", parts); err == nil {
			t.Fatal("invalid completion parts accepted")
		}
	}
	if _, err := store.PresignGet(ctx, "key", 8*24*time.Hour); err == nil {
		t.Fatal("invalid presign expiry accepted")
	}
	if err := store.DeletePrefix(ctx, ""); err == nil {
		t.Fatal("empty delete prefix accepted")
	}
	if _, err := store.PutStream(ctx, "key", strings.NewReader(""), -1); err == nil {
		t.Fatal("negative size limit accepted")
	}
}

func TestReadPartPreservesTruncatedSourceError(t *testing.T) {
	buf := make([]byte, 10)
	n, err := readPart(context.Background(), io.MultiReader(strings.NewReader("abc"), errorReader{err: io.ErrUnexpectedEOF}), buf)
	if n != 3 || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("read n=%d err=%v", n, err)
	}
	n, err = readPart(context.Background(), strings.NewReader("abc"), buf)
	if n != 3 || !errors.Is(err, io.EOF) {
		t.Fatalf("normal final part n=%d err=%v", n, err)
	}
}

func TestDeletePrefixPaginatesAndReportsPartialFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			var lists, deletes int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				if r.Method == http.MethodGet {
					lists++
					if r.URL.Query().Get("prefix") != "workspaces/1/" {
						t.Error("list lost delete prefix")
					}
					if lists == 1 {
						fmt.Fprint(w, `<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>next</NextContinuationToken><Contents><Key>workspaces/1/a</Key></Contents></ListBucketResult>`)
					} else {
						if r.URL.Query().Get("continuation-token") != "next" {
							t.Error("pagination token was lost")
						}
						fmt.Fprint(w, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>workspaces/1/b</Key></Contents></ListBucketResult>`)
					}
					return
				}
				if r.Method == http.MethodPost && r.URL.Query().Has("delete") {
					deletes++
					if fail {
						fmt.Fprint(w, `<DeleteResult><Error><Key>workspaces/1/a</Key><Code>AccessDenied</Code><Message>denied</Message></Error></DeleteResult>`)
					} else {
						fmt.Fprint(w, `<DeleteResult/>`)
					}
					return
				}
				t.Errorf("unexpected request %s %s", r.Method, r.URL)
			}))
			defer server.Close()
			client := s3.NewFromConfig(aws.Config{Region: "us-east-1", Credentials: aws.AnonymousCredentials{}}, func(o *s3.Options) {
				o.BaseEndpoint = aws.String(server.URL)
				o.UsePathStyle = true
			})
			err := New(client, "bucket").DeletePrefix(context.Background(), "workspaces/1/")
			if fail {
				if err == nil || !strings.Contains(err.Error(), "AccessDenied") || lists != 1 || deletes != 1 {
					t.Fatalf("partial failure: lists=%d deletes=%d err=%v", lists, deletes, err)
				}
			} else if err != nil || lists != 2 || deletes != 2 {
				t.Fatalf("pagination: lists=%d deletes=%d err=%v", lists, deletes, err)
			}
		})
	}
}

type readOnly struct{ io.Reader }
