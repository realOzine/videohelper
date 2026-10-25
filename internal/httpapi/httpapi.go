package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/pipeline"
	"github.com/ozine/videohelper/internal/platform/authn"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/search"
	"github.com/ozine/videohelper/internal/workspace"
)

type Config struct {
	CORSOrigins []string
	PresignTTL  time.Duration
}

type Deps struct {
	Authn     *authn.Verifier
	Workspace *workspace.Service
	Library   *library.Service
	Pipeline  *pipeline.Service
	Search    *search.Service
	Blob      *blob.Store
	Config    Config
	Ready     func(context.Context) error
}

type api struct{ Deps }
type endpoint func(*http.Request) (int, any, error)

func New(d Deps) http.Handler {
	if d.Config.PresignTTL == 0 {
		d.Config.PresignTTL = time.Hour
	}
	a := api{Deps: d}
	private := http.NewServeMux()
	for pattern, fn := range map[string]endpoint{
		"GET /v1/me":       a.me,
		"GET /v1/projects": a.projects, "POST /v1/projects": a.createProject, "DELETE /v1/projects/{id}": a.deleteProject,
		"PUT /v1/projects/{id}/videos/{video_id}": a.addVideo, "DELETE /v1/projects/{id}/videos/{video_id}": a.removeVideo,
		"POST /v1/uploads": a.startUpload, "POST /v1/videos/{id}/complete": a.completeUpload,
		"GET /v1/videos": a.videos, "GET /v1/videos/{id}": a.video, "DELETE /v1/videos/{id}": a.deleteVideo,
		"POST /v1/videos/{id}/restore": a.restoreVideo, "POST /v1/videos/{id}/retry": a.retryVideo, "GET /v1/videos/{id}/playback": a.playback,
		"POST /v1/search": a.search,
	} {
		private.Handle(pattern, serve(fn))
	}
	private.Handle("/", serve(notFound))
	mux := http.NewServeMux()
	mux.Handle("GET /healthz", serve(func(*http.Request) (int, any, error) { return http.StatusOK, map[string]string{"status": "ok"}, nil }))
	mux.Handle("GET /readyz", serve(func(r *http.Request) (int, any, error) {
		if d.Ready == nil {
			return 0, nil, apperr.New(apperr.Unavailable, "unavailable", "服务尚未就绪")
		}
		if err := d.Ready(r.Context()); err != nil {
			return 0, nil, apperr.Wrap(apperr.Unavailable, "unavailable", err)
		}
		return http.StatusOK, map[string]string{"status": "ok"}, nil
	}))
	mux.Handle("/v1/", a.authenticate(private))
	mux.Handle("/", serve(notFound))
	return requestID(accessLog(recoverPanic(cors(d.Config.CORSOrigins, mux))))
}

func notFound(*http.Request) (int, any, error) {
	return 0, nil, apperr.New(apperr.NotFound, "not_found", "资源不存在")
}

func serve(fn endpoint) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		status, body, err := fn(r)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, status, body)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			writeError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
	}
	w.WriteHeader(status)
	if body != nil {
		w.Write(append(data, '\n'))
	}
}

func writeError(w http.ResponseWriter, err error) {
	kind := apperr.KindOf(err)
	status, code, message := http.StatusInternalServerError, "internal", "服务器内部错误"
	switch kind {
	case apperr.Invalid:
		status, code, message = 400, "invalid_argument", "参数不合法"
	case apperr.Unauthenticated:
		status, code, message = 401, "unauthenticated", "请先登录"
	case apperr.Forbidden:
		status, code, message = 403, "permission_denied", "没有访问权限"
	case apperr.NotFound:
		status, code, message = 404, "not_found", "资源不存在"
	case apperr.Conflict:
		status, code, message = 409, "conflict", "当前状态不允许此操作"
	case apperr.TooLarge:
		status, code, message = 413, "payload_too_large", "内容超过大小限制"
	case apperr.Quota:
		status, code, message = 429, "quota_exceeded", "已超出套餐额度"
	case apperr.Unavailable:
		status, code, message = 503, "unavailable", "服务暂时不可用"
	}
	var public *apperr.Error
	if kind != apperr.Internal && errors.As(err, &public) {
		message = public.Error()
		if kind == apperr.Conflict && (public.Code == "not_ready" || public.Code == "media_not_ready") {
			code = "not_ready"
		}
	}
	if status == 401 {
		w.Header().Set("WWW-Authenticate", "Bearer")
	}
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func decode(r *http.Request, out any, optional bool) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		if optional && errors.Is(err, io.EOF) {
			return nil
		}
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			return apperr.New(apperr.TooLarge, "payload_too_large", "请求体超过 1 MiB 限制")
		}
		return apperr.New(apperr.Invalid, "invalid_argument", "JSON 请求体不合法")
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			return apperr.New(apperr.TooLarge, "payload_too_large", "请求体超过 1 MiB 限制")
		}
		return apperr.New(apperr.Invalid, "invalid_argument", "请求体只能包含一个 JSON 对象")
	}
	return nil
}

func (a api) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeError(w, apperr.New(apperr.Unauthenticated, "unauthenticated", "需要 Bearer 令牌"))
			return
		}
		if a.Authn == nil || a.Workspace == nil {
			writeError(w, apperr.New(apperr.Unavailable, "unavailable", "认证服务尚未就绪"))
			return
		}
		claims, err := a.Authn.Verify(r.Context(), parts[1])
		if err != nil {
			writeError(w, err)
			return
		}
		requested := int64(0)
		if raw := r.Header.Get("X-Workspace-ID"); raw != "" {
			requested, err = parseID(raw)
			if err != nil {
				writeError(w, err)
				return
			}
		}
		p, err := a.Workspace.Resolve(r.Context(), claims, requested)
		if err != nil {
			writeError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(workspace.WithPrincipal(r.Context(), p)))
	})
}

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		bad := len(id) > 256 || strings.IndexFunc(id, func(char rune) bool { return char < 32 || char == 127 }) >= 0
		if id == "" || bad {
			id = rand.Text()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := mlclient.WithRequestID(r.Context(), id)
		if trace := r.Header.Get("traceparent"); trace != "" {
			ctx = mlclient.WithTraceparent(ctx, trace)
		}
		if bad {
			writeError(w, apperr.New(apperr.Invalid, "invalid_argument", "请求 ID 不合法"))
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.ResponseWriter.WriteHeader(status)
	}
}
func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(data)
}

func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		record := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(record, r)
		slog.InfoContext(r.Context(), "HTTP request", "method", r.Method, "path", r.URL.Path, "status", record.status, "duration_ms", time.Since(start).Milliseconds(), "request_id", mlclient.RequestID(r.Context()))
	})
}

func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recover() != nil {
				slog.ErrorContext(r.Context(), "HTTP handler panic", "request_id", mlclient.RequestID(r.Context()))
				writeError(w, apperr.New(apperr.Internal, "internal", "服务器内部错误"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func cors(origins []string, next http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, origin := range origins {
		allowed[origin] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Add("Vary", "Origin")
			if !allowed[origin] {
				writeError(w, apperr.New(apperr.Forbidden, "permission_denied", "请求来源不在允许列表中"))
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Workspace-ID, X-Request-ID")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
