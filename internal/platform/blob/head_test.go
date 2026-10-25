package blob

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestHeadAndAbort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodHead && r.URL.Path == "/bucket/exists":
			w.Header().Set("Content-Length", "123")
			w.Header().Set("Content-Type", "video/mp4")
		case r.Method == http.MethodHead:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodDelete && r.URL.Query().Get("uploadId") == "missing":
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `<Error><Code>NoSuchUpload</Code></Error>`)
		case r.Method == http.MethodDelete && r.URL.Query().Get("uploadId") == "upload":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
	}))
	defer server.Close()
	client := s3.NewFromConfig(aws.Config{Region: "us-east-1", Credentials: aws.AnonymousCredentials{}}, func(o *s3.Options) { o.BaseEndpoint = aws.String(server.URL); o.UsePathStyle = true })
	store := New(client, "bucket")
	if info, err := store.Head(context.Background(), "exists"); err != nil || info.Size != 123 || info.ContentType != "video/mp4" {
		t.Fatalf("Head: %+v %v", info, err)
	}
	if _, err := store.Head(context.Background(), "absent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing object: %v", err)
	}
	for _, id := range []string{"upload", "missing"} {
		if err := store.AbortMultipart(context.Background(), "key", id); err != nil {
			t.Fatalf("abort %q: %v", id, err)
		}
	}
}
