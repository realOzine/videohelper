package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ozine/videohelper/internal/apperr"
)

func TestPublicRoutesRequestIDAndCORS(t *testing.T) {
	handler := New(Deps{Config: Config{CORSOrigins: []string{"https://app.example.test"}}})
	request := func(method, path string, headers map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, nil)
		for key, value := range headers {
			r.Header.Set(key, value)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/healthz", map[string]string{"X-Request-ID": "keep-this-id"}); w.Code != 200 || w.Header().Get("X-Request-ID") != "keep-this-id" {
		t.Fatalf("health/request ID: %d %v", w.Code, w.Header())
	}
	if w := request("GET", "/readyz", nil); w.Code != 503 || !strings.Contains(w.Body.String(), "unavailable") {
		t.Fatalf("invented readiness: %d %s", w.Code, w.Body)
	}
	if w := request("OPTIONS", "/v1/search", map[string]string{"Origin": "https://app.example.test", "Access-Control-Request-Method": "POST"}); w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "https://app.example.test" {
		t.Fatalf("CORS preflight: %d %v", w.Code, w.Header())
	}
	if w := request("OPTIONS", "/v1/search", map[string]string{"Origin": "https://app.example.test.evil.test"}); w.Code != 403 || w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("origin whitelist bypass: %d %v", w.Code, w.Header())
	}
	if w := request("GET", "/v1/me", nil); w.Code != 401 || w.Header().Get("WWW-Authenticate") != "Bearer" || w.Header().Get("X-Request-ID") == "" {
		t.Fatalf("missing authentication: %d %s", w.Code, w.Body)
	}
	if w := request("POST", "/v1/search/models", nil); w.Code != 401 {
		t.Fatalf("model catalog missing authentication: %d %s", w.Code, w.Body)
	}
	if w := request("GET", "/healthz", map[string]string{"X-Request-ID": strings.Repeat("x", 257)}); w.Code != 400 || len(w.Header().Get("X-Request-ID")) > 256 {
		t.Fatalf("unsafe request ID: %d %v", w.Code, w.Header())
	}
	ready := New(Deps{Ready: func(context.Context) error { return errors.New("private readiness password") }})
	w := httptest.NewRecorder()
	ready.ServeHTTP(w, httptest.NewRequest("GET", "/readyz", nil))
	if w.Code != 503 || strings.Contains(w.Body.String(), "password") {
		t.Fatalf("readiness leaked cause: %d %s", w.Code, w.Body)
	}
}

func TestSafeErrorsPanicAndStrictBodies(t *testing.T) {
	for _, tt := range []struct {
		kind   apperr.Kind
		status int
		code   string
	}{
		{apperr.Invalid, 400, "invalid_argument"}, {apperr.Unauthenticated, 401, "unauthenticated"}, {apperr.Forbidden, 403, "permission_denied"}, {apperr.NotFound, 404, "not_found"}, {apperr.Conflict, 409, "conflict"}, {apperr.TooLarge, 413, "payload_too_large"}, {apperr.Quota, 429, "quota_exceeded"}, {apperr.Unavailable, 503, "unavailable"}, {apperr.Internal, 500, "internal"},
	} {
		w := httptest.NewRecorder()
		writeError(w, apperr.Wrap(tt.kind, "internal-service-code", errors.New("secret-token")))
		if w.Code != tt.status || !strings.Contains(w.Body.String(), `"code":"`+tt.code+`"`) || strings.Contains(w.Body.String(), "secret-token") {
			t.Fatalf("error mapping: %d %s", w.Code, w.Body)
		}
	}
	w := httptest.NewRecorder()
	requestID(recoverPanic(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("private panic content") }))).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 500 || strings.Contains(w.Body.String(), "private") || w.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("panic response: %d %s", w.Code, w.Body)
	}
	handler := serve(func(r *http.Request) (int, any, error) {
		var body struct {
			Name string `json:"name"`
		}
		if err := decode(r, &body, false); err != nil {
			return 0, nil, err
		}
		return 200, body, nil
	})
	for _, tt := range []struct {
		body   string
		status int
	}{
		{`{"name":"项目","extra":true}`, 400}, {`{"name":"项目"} {}`, 400}, {`{"name":"` + strings.Repeat("x", 1<<20) + `"}`, 413},
	} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader(tt.body)))
		if w.Code != tt.status {
			t.Fatalf("body status=%d want%d: %s", w.Code, tt.status, w.Body)
		}
	}
}

func TestStringIDsAndTextOnlySearchBody(t *testing.T) {
	for _, raw := range []string{"", "0", "-1", "+1", "01", "1.2", "1x", "9223372036854775808"} {
		if _, err := parseID(raw); apperr.KindOf(err) != apperr.Invalid {
			t.Fatalf("accepted malformed ID %q: %v", raw, err)
		}
	}
	for _, body := range []string{`{"query":"中文","image":{"uri":"s3://bucket/key"}}`, `{"query":"中文","video_ids":[1]}`, `{"query":"中文","project_id":1}`} {
		w := httptest.NewRecorder()
		serve((api{}).search).ServeHTTP(w, httptest.NewRequest("POST", "/v1/search", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatalf("image/numeric ID reached nil search service: %d %s", w.Code, w.Body)
		}
	}
	for _, body := range []string{`{"query":"中文"}`, `{"time_range":{"start_ms":0,"end_ms":1000}}`, `{"space_overrides":{"visual":"model@r1"}}`, `{"video_ids":[1]}`, `{"project_id":1}`, `{"project_id":"0"}`, `{"video_ids":["bad"]}`, `{} {}`} {
		w := httptest.NewRecorder()
		serve((api{}).searchModels).ServeHTTP(w, httptest.NewRequest("POST", "/v1/search/models", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatalf("invalid model catalog body reached nil search service: %d %s", w.Code, w.Body)
		}
	}
}

func TestSearchScopePreservesEmptySelection(t *testing.T) {
	for _, ids := range [][]string{nil, {}, {"2", "1"}} {
		scope, err := searchScope(nil, ids)
		if err != nil || (scope.VideoIDs == nil) != (ids == nil) || len(scope.VideoIDs) != len(ids) {
			t.Fatalf("scope lost empty/all distinction: %+v %v", scope, err)
		}
		for i, raw := range ids {
			id, _ := parseID(raw)
			if scope.VideoIDs[i] != id {
				t.Fatalf("scope changed video ID: %+v", scope)
			}
		}
	}
}
