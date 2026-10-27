// Package server adapts the personal desktop library to its existing web UI.
package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/desktop/processor"
	"github.com/ozine/videohelper/internal/desktop/store"
	"github.com/ozine/videohelper/internal/timeline"
)

type Deps struct {
	Store     *store.Store
	Processor func() *processor.Processor
	State     func() any
	Retry     func() error
	Dev       bool
}

type Server struct {
	Deps
	Base, Token string
	mediaSecret string
	server      *http.Server
	host        string
}

func Start(d Deps) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{Deps: d, Base: "http://" + ln.Addr().String(), host: ln.Addr().String(), Token: rand.Text(), mediaSecret: rand.Text()}
	s.server = &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		if err := s.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("desktop HTTP server stopped", "error", err)
		}
	}()
	return s, nil
}

func (s *Server) Close(ctx context.Context) error { return s.server.Shutdown(ctx) }

type endpoint func(*http.Request) (any, error)

func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	for pattern, fn := range map[string]endpoint{
		"GET /v1/desktop/runtime": func(*http.Request) (any, error) { return s.State(), nil },
		"POST /v1/desktop/runtime/retry": func(*http.Request) (any, error) {
			if s.Retry == nil {
				return nil, store.ErrConflict
			}
			return nil, s.Retry()
		},
		"GET /v1/projects": s.projects, "POST /v1/projects": s.createProject, "DELETE /v1/projects/{id}": s.deleteProject,
		"PUT /v1/projects/{id}/videos/{video_id}": s.addProjectVideo, "DELETE /v1/projects/{id}/videos/{video_id}": s.removeProjectVideo,
		"GET /v1/videos": s.videos, "GET /v1/videos/{id}": s.video, "DELETE /v1/videos/{id}": s.deleteVideo,
		"POST /v1/videos/{id}/restore": s.restoreVideo, "POST /v1/videos/{id}/retry": s.retryVideo, "POST /v1/videos/{id}/cancel": s.cancelVideo,
		"GET /v1/videos/{id}/playback": s.playback,
		"POST /v1/imports/file":        s.importFile, "POST /v1/imports/url": s.importURL,
		"POST /v1/search": s.search, "POST /v1/search/models": s.models,
	} {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if len(r.Header.Values("X-Desktop-Token")) != 1 || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Desktop-Token")), []byte(s.Token)) != 1 {
				writeError(w, http.StatusForbidden, "forbidden", "This connection is no longer available. Reopen VideoHelper.")
				return
			}
			body, err := fn(r)
			if err != nil {
				writeFailure(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if body == nil {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if err := json.NewEncoder(w).Encode(body); err != nil {
				slog.Debug("desktop response ended", "error", err)
			}
		})
	}
	mux.HandleFunc("GET /media/{id}/{cap}/{key...}", s.media)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Host != s.host {
			writeError(w, 403, "forbidden", "Invalid local host")
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" {
			if origin != "mygo://localhost" && origin != "http://mygo.localhost" && !(s.Dev && origin == "http://127.0.0.1:5173") {
				writeError(w, 403, "forbidden", "Invalid local origin")
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Headers", "X-Desktop-Token, Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, PUT, DELETE, OPTIONS")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		mux.ServeHTTP(w, r)
	})
}

func decode(r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return store.ErrInvalid
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return store.ErrInvalid
	}
	return nil
}

func id(raw string) (int64, error) {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 || strconv.FormatInt(n, 10) != raw {
		return 0, store.ErrInvalid
	}
	return n, nil
}

func optionalID(raw string) (*int64, error) {
	if raw == "" {
		return nil, nil
	}
	n, err := id(raw)
	return &n, err
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrInvalid):
		writeError(w, 400, "invalid", err.Error())
	case errors.Is(err, store.ErrNotFound), errors.Is(err, os.ErrNotExist):
		writeError(w, 404, "not_found", "Video or project not found")
	case errors.Is(err, store.ErrConflict), errors.Is(err, store.ErrStale):
		writeError(w, 409, "conflict", err.Error())
	case apperr.KindOf(err) == apperr.Unavailable:
		writeError(w, 503, "not_ready", err.Error())
	case apperr.KindOf(err) == apperr.Invalid:
		writeError(w, 400, "invalid", err.Error())
	case errors.Is(err, context.Canceled):
		writeError(w, 499, "cancelled", "Operation canceled")
	default:
		slog.Error("desktop request failed", "error", err)
		writeError(w, 500, "internal", "Operation failed; check the local application log")
	}
}

func (s *Server) engine() (*processor.Processor, error) {
	if s.Processor != nil {
		if p := s.Processor(); p != nil {
			return p, nil
		}
	}
	return nil, apperr.New(apperr.Unavailable, "not_ready", "Local models are still preparing")
}

type thumbnail struct {
	URL string `json:"url"`
	X   int    `json:"x"`
	Y   int    `json:"y"`
	W   int    `json:"w"`
	H   int    `json:"h"`
}

type videoDTO struct {
	store.Video
	Thumbnail *thumbnail `json:"thumbnail"`
}

func (s *Server) public(v store.Video) videoDTO {
	if v.DeletedAt != nil {
		v.Status = "deleted"
		return videoDTO{Video: v}
	}
	return videoDTO{Video: v, Thumbnail: s.thumb(v, 0)}
}

func (s *Server) mediaCap(videoID int64, key string) string {
	h := hmac.New(sha256.New, []byte(s.mediaSecret))
	h.Write([]byte(strconv.FormatInt(videoID, 10) + "\x00" + key))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Server) mediaURL(v store.Video, key string) string {
	return s.Base + "/media/" + strconv.FormatInt(v.ID, 10) + "/" + s.mediaCap(v.ID, key) + "/" + key
}

func (s *Server) thumb(v store.Video, cell int) *thumbnail {
	if v.Thumbs == nil || v.NormalizedVideoKey == "" {
		return nil
	}
	sprite, x, y := v.Thumbs.Tile(cell)
	if sprite < 0 {
		return nil
	}
	key := path.Join(path.Dir(v.NormalizedVideoKey), "sprites", strconv.Itoa(sprite)+".jpg")
	return &thumbnail{URL: s.mediaURL(v, key), X: x, Y: y, W: v.Thumbs.TileW, H: v.Thumbs.TileH}
}

func (s *Server) media(w http.ResponseWriter, r *http.Request) {
	n, err := id(r.PathValue("id"))
	if err != nil {
		writeFailure(w, err)
		return
	}
	key := r.PathValue("key")
	if !hmac.Equal([]byte(r.PathValue("cap")), []byte(s.mediaCap(n, key))) {
		writeError(w, 403, "forbidden", "Invalid media link")
		return
	}
	v, err := s.Store.Video(r.Context(), n)
	if err != nil {
		writeFailure(w, err)
		return
	}
	if v.DeletedAt != nil || !strings.HasPrefix(key, "videos/"+strconv.FormatInt(n, 10)+"/") {
		writeError(w, 404, "not_found", "Media not found")
		return
	}
	f, err := s.Store.OpenFile(key)
	if err != nil {
		writeFailure(w, err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		writeFailure(w, err)
		return
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

type queryJSON struct {
	Query          string             `json:"query"`
	ProjectID      string             `json:"project_id"`
	VideoIDs       []string           `json:"video_ids"`
	TimeRange      *timeline.Span     `json:"time_range"`
	Limit          int                `json:"limit"`
	SpaceOverrides map[string]string  `json:"space_overrides"`
	ChannelWeights map[string]float64 `json:"channel_weights"`
}

func (q queryJSON) query() (processor.Query, error) {
	project, err := optionalID(q.ProjectID)
	if err != nil {
		return processor.Query{}, err
	}
	var ids []int64
	if q.VideoIDs != nil {
		ids = make([]int64, len(q.VideoIDs))
	}
	for i, raw := range q.VideoIDs {
		if ids[i], err = id(raw); err != nil {
			return processor.Query{}, err
		}
	}
	return processor.Query{Text: q.Query, ProjectID: project, VideoIDs: ids, TimeRange: q.TimeRange, Limit: q.Limit, ChannelWeights: q.ChannelWeights, SpaceOverride: q.SpaceOverrides}, nil
}
