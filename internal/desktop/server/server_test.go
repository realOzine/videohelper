package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ozine/videohelper/internal/desktop/store"
)

func startTest(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	db, err := store.Open(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := Start(Deps{Store: db, State: func() any { return map[string]any{"ready": false} }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		s.Close(ctx)
	})
	return s, db
}

func request(t *testing.T, s *Server, method, path, body string, mutate func(*http.Request)) *http.Response {
	t.Helper()
	r, err := http.NewRequest(method, s.Base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("X-Desktop-Token", s.Token)
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if mutate != nil {
		mutate(r)
	}
	res, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func TestPrivateLoopbackAndStrictJSON(t *testing.T) {
	s, _ := startTest(t)
	for _, tc := range []struct {
		name   string
		mutate func(*http.Request)
	}{
		{"missing capability", func(r *http.Request) { r.Header.Del("X-Desktop-Token") }},
		{"rebound host", func(r *http.Request) { r.Host = "evil.example" }},
		{"external origin", func(r *http.Request) { r.Header.Set("Origin", "https://evil.example") }},
		{"production dev origin", func(r *http.Request) { r.Header.Set("Origin", "http://127.0.0.1:5173") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if res := request(t, s, "GET", "/v1/projects", "", tc.mutate); res.StatusCode != 403 {
				t.Fatal(res.Status)
			}
		})
	}
	res := request(t, s, "OPTIONS", "/v1/projects", "", func(r *http.Request) { r.Header.Del("X-Desktop-Token"); r.Header.Set("Origin", "mygo://localhost") })
	if res.StatusCode != 204 || res.Header.Get("Access-Control-Allow-Origin") != "mygo://localhost" {
		t.Fatalf("preflight: %s", res.Status)
	}
	for _, body := range []string{`{"name":"A","extra":1}`, `{"name":"A"} {"name":"B"}`, strings.Repeat("x", 1<<20+1)} {
		if res := request(t, s, "POST", "/v1/projects", body, nil); res.StatusCode != 400 {
			t.Fatal(res.Status)
		}
	}
	if res := request(t, s, "POST", "/v1/projects", `{"name":"A"}`, nil); res.StatusCode != 200 {
		t.Fatal(res.Status)
	}
	res = request(t, s, "POST", "/v1/search", `{"query":"hello"}`, nil)
	if res.StatusCode != 503 {
		t.Fatalf("unready search: %s", res.Status)
	}
}

func TestMediaRangeCapabilityAndDeletion(t *testing.T) {
	s, db := startTest(t)
	input := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(input, []byte("0123456789"), 0600); err != nil {
		t.Fatal(err)
	}
	v, err := db.ImportFile(t.Context(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateMedia(t.Context(), v.ID, store.MediaInfo{DurationMs: 1000, HasVideo: true, NormalizedVideoKey: v.SourceKey}); err != nil {
		t.Fatal(err)
	}
	res := request(t, s, "GET", "/v1/videos/"+strconv.FormatInt(v.ID, 10)+"/playback", "", nil)
	var playback struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(res.Body).Decode(&playback); err != nil {
		t.Fatal(err)
	}
	path := strings.TrimPrefix(playback.URL, s.Base)
	res = request(t, s, "GET", path, "", func(r *http.Request) { r.Header.Del("X-Desktop-Token"); r.Header.Set("Range", "bytes=3-6") })
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 206 || string(b) != "3456" || res.Header.Get("Content-Range") != "bytes 3-6/10" {
		t.Fatalf("range %s: %s", res.Status, b)
	}
	bad := strings.Replace(path, "/media/", "/media/999/", 1)
	if res := request(t, s, "GET", bad, "", nil); res.StatusCode == 200 || res.StatusCode == 206 {
		t.Fatal("altered media URL accepted")
	}
	if err := db.DeleteVideo(t.Context(), v.ID); err != nil {
		t.Fatal(err)
	}
	if res := request(t, s, "GET", path, "", nil); res.StatusCode != 404 {
		t.Fatalf("deleted media accessible: %s", res.Status)
	}
}

func TestLibraryCursorAndRestore(t *testing.T) {
	s, db := startTest(t)
	for range 3 {
		if _, err := db.CreateImport(t.Context(), "https://example.com/clip.mp4", "direct", "clip", nil); err != nil {
			t.Fatal(err)
		}
	}
	res := request(t, s, "GET", "/v1/videos?limit=2", "", nil)
	var page struct {
		Videos []videoDTO `json:"videos"`
		Cursor *string    `json:"next_cursor"`
	}
	if err := json.NewDecoder(res.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Videos) != 2 || page.Cursor == nil {
		t.Fatalf("first page: %+v", page)
	}
	res = request(t, s, "GET", "/v1/videos?limit=2&cursor="+*page.Cursor, "", nil)
	if err := json.NewDecoder(res.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Videos) != 1 || page.Cursor != nil {
		t.Fatalf("last page: %+v", page)
	}
	n := page.Videos[0].ID
	if err := db.DeleteVideo(t.Context(), n); err != nil {
		t.Fatal(err)
	}
	res = request(t, s, "GET", "/v1/videos?status=deleted", "", nil)
	if err := json.NewDecoder(res.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if len(page.Videos) != 1 || page.Videos[0].Status != "deleted" {
		t.Fatalf("trash: %+v", page)
	}
	if res := request(t, s, "POST", "/v1/videos/"+strconv.FormatInt(n, 10)+"/restore", "", nil); res.StatusCode != 200 {
		t.Fatal(res.Status)
	}
}
