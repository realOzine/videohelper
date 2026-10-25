package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/workspace"
)

func TestLocalCommandArguments(t *testing.T) {
	for _, item := range []struct {
		args []string
		role config.Role
	}{
		{[]string{"api"}, config.RoleAPI}, {[]string{"worker"}, config.RoleWorker}, {[]string{"local-upload", "a b.mp4"}, config.RoleWorker},
		{[]string{"local-search", "red screen"}, config.RoleEval}, {[]string{"local-video", "42"}, config.RoleMigrate},
	} {
		role, err := commandRole(item.args)
		if err != nil || role != item.role {
			t.Fatalf("%v => %s, %v", item.args, role, err)
		}
	}
	for _, args := range [][]string{{"worker", "extra"}, {"local-upload"}, {"local-upload", " "}, {"local-search", ""}, {"local-video", "0"}, {"local-video", "-1"}, {"local-video", "9223372036854775808"}, {"local-search", "one", "two"}} {
		if _, err := commandRole(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestUploadPartsStreamsBoundedSectionsAndRetainsETags(t *testing.T) {
	bodies := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.ContentLength <= 0 || len(r.TransferEncoding) != 0 {
			t.Errorf("unexpected upload framing: %s length=%d", r.Method, r.ContentLength)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
		}
		if int64(len(body)) != r.ContentLength {
			t.Error("incorrect content length")
		}
		bodies <- string(body)
		w.Header().Set("ETag", "\"etag-"+strings.TrimPrefix(r.URL.Path, "/")+"\"")
	}))
	defer server.Close()
	client := commandHTTPClient()
	defer client.CloseIdleConnections()
	plan := library.UploadPlan{PartSize: 4, Parts: []blob.PartURL{{N: 1, URL: server.URL + "/1"}, {N: 2, URL: server.URL + "/2"}}}
	parts, err := uploadParts(context.Background(), client, strings.NewReader("abcdefg-extra"), 7, plan)
	if err != nil {
		t.Fatal(err)
	}
	received := []string{<-bodies, <-bodies}
	if !slices.Equal(received, []string{"abcd", "efg"}) || len(parts) != 2 || parts[1].ETag != "\"etag-2\"" {
		t.Fatalf("bodies=%v parts=%+v", received, parts)
	}
}

func TestUploadPartFailuresAndCancellation(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTemporaryRedirect, http.StatusOK} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "/must-not-follow")
				w.WriteHeader(status)
			}))
			defer server.Close()
			client := commandHTTPClient()
			defer client.CloseIdleConnections()
			plan := library.UploadPlan{PartSize: 1, Parts: []blob.PartURL{{N: 1, URL: server.URL}}}
			if _, err := uploadParts(context.Background(), client, strings.NewReader("a"), 1, plan); err == nil {
				t.Fatal("failed response or missing ETag accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	plan := library.UploadPlan{PartSize: 1, Parts: []blob.PartURL{{N: 1, URL: "http://127.0.0.1:1/private?signature=secret"}}}
	client := commandHTTPClient()
	defer client.CloseIdleConnections()
	_, err := uploadParts(ctx, client, strings.NewReader("a"), 1, plan)
	if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("cancellation or signed URL leak: %v", err)
	}
}

func TestLocalUploadRejectsDirectoryAndEmptyFile(t *testing.T) {
	directory := t.TempDir()
	empty := filepath.Join(directory, "empty.mp4")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{directory, empty} {
		if _, err := localUpload(context.Background(), nil, workspace.Principal{}, path); err == nil {
			t.Fatalf("accepted %s", path)
		}
	}
}
