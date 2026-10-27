package mlclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/config"
)

const testSpace = "test-space@r1"
const testModel = "test-asr@r1"

var testFingerprint = "sha256:" + strings.Repeat("a", 64)

// These tiny synthetic capabilities are only for contract tests, not model claims.
func testMetadata() SpacesResponse {
	span := 2500
	return SpacesResponse{
		Spaces:       []SpaceInfo{{ID: testSpace, Dim: 3, Metric: "cosine", QueryInputs: []string{"text"}, DocInputs: []string{"text", "video", "image"}, MaxSpanMs: &span, MaxBatch: 3, Fingerprint: testFingerprint}},
		Transcribers: []TranscriberInfo{{ID: testModel, Languages: []string{"zh"}, MaxWindowMs: 3000, Fingerprint: testFingerprint}},
	}
}

func testClient(endpoint string) *Client {
	return New(config.MLConfig{Endpoints: []string{endpoint}, Token: "test-only-token", QueryTimeout: time.Second, EmbedTimeout: time.Second, TranscribeTimeout: time.Second},
		func(space string) (string, bool) { return endpoint, space == testSpace })
}

func query() EmbedRequest {
	return EmbedRequest{Space: testSpace, Role: "query", Inputs: []EmbedInput{{Text: "测试查询"}}}
}

func embedding() EmbedResponse {
	return EmbedResponse{Space: testSpace, Fingerprint: testFingerprint, Dim: 3, Vectors: [][]float32{{1, 0, 0}}}
}

func transcript() TranscribeResponse {
	return TranscribeResponse{Model: testModel, Fingerprint: testFingerprint, Language: "zh", DurationMs: 2500,
		Sentences: []Sentence{{StartMs: 1000, EndMs: 1500, Text: "你好", Words: []Word{{W: "你", S: 1000, E: 1200}, {W: "好", S: 1200, E: 1500}}},
			{StartMs: 2000, EndMs: 2500, Text: "世界", Words: []Word{}}}}
}

func transcription() TranscribeRequest {
	return TranscribeRequest{Model: testModel, URI: "local://media/video.mp4", Language: "zh", StartMs: 1000, EndMs: 3000}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}

func TestEmbedWireContractAndHeaders(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		remaining, err := strconv.Atoi(r.Header.Get("X-Timeout-Ms"))
		if err != nil || remaining < 1 || remaining > 500 || r.Header.Get("X-Internal-Token") != "test-only-token" || r.Header.Get("X-Request-ID") != "request-123" || r.Header.Get("traceparent") != "test-trace" {
			t.Errorf("unexpected request headers: %v", r.Header)
		}
		switch r.URL.Path {
		case "/v1/spaces":
			writeJSON(w, testMetadata())
		case "/v1/embed":
			if r.Method != http.MethodPost {
				t.Errorf("method: %s", r.Method)
			}
			var body struct {
				Inputs []map[string]json.RawMessage `json:"inputs"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Inputs) != 3 {
				t.Errorf("decode request: %v, %+v", err, body)
			}
			var image map[string]any
			json.Unmarshal(body.Inputs[2]["image"], &image)
			if len(image) != 1 || image["uri"] != "local://media/image.jpg" {
				t.Errorf("image reference acquired timestamp fields: %v", image)
			}
			response := embedding()
			response.Vectors = append(response.Vectors, []float32{0, 1, 0}, []float32{0, 0, 1})
			writeJSON(w, response)
		default:
			t.Errorf("unexpected route: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(WithTraceparent(WithRequestID(context.Background(), "request-123"), "test-trace"), 500*time.Millisecond)
	defer cancel()
	req := EmbedRequest{Space: testSpace, Role: "document", Inputs: []EmbedInput{{Text: "转录原文"}, {Video: &MediaRef{URI: "local://media/video.mp4", StartMs: 0, EndMs: 2500}}, {Image: &ImageRef{URI: "local://media/image.jpg"}}}}
	response, err := testClient(server.URL).Embed(ctx, req)
	if err != nil || len(response.Vectors) != 3 || requests.Load() != 2 {
		t.Fatalf("Embed: %+v, %v, requests=%d", response, err, requests.Load())
	}
}

func TestMissingLocalRouteIsRetryable(t *testing.T) {
	client := New(config.MLConfig{Token: "test"}, func(string) (string, bool) { return "", false })
	_, err := client.Embed(context.Background(), query())
	var classified *Error
	if !errors.As(err, &classified) || !classified.Retryable || classified.Status != 0 || classified.Code != "space_unavailable" || apperr.KindOf(err) != apperr.Unavailable {
		t.Fatalf("transient registry removal became a permanent model error: %v", err)
	}
}

func TestDiscoveryPartialAvailabilityAndRefresh(t *testing.T) {
	var changed atomic.Bool
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/spaces" {
			metadata := testMetadata()
			if changed.Load() {
				metadata.Spaces[0].Fingerprint = "sha256:" + strings.Repeat("b", 64)
			}
			writeJSON(w, metadata)
		} else {
			response := embedding()
			if changed.Load() {
				response.Fingerprint = "sha256:" + strings.Repeat("b", 64)
			}
			writeJSON(w, response)
		}
	}))
	defer healthy.Close()
	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer unavailable.Close()
	client := testClient(healthy.URL)
	client.cfg.Endpoints = []string{unavailable.URL, healthy.URL}
	result := client.Discover(context.Background())
	if len(result.Endpoints) != 1 || len(result.Errors) != 1 || result.Errors[unavailable.URL] == nil {
		t.Fatalf("partial discovery: %+v", result)
	}
	// Returned metadata may be used and modified by callers without altering the cache.
	metadata := result.Endpoints[healthy.URL]
	metadata.Spaces[0].Dim = 999
	metadata.Spaces[0].QueryInputs[0] = "image"
	*metadata.Spaces[0].MaxSpanMs = 1
	if _, err := client.Embed(context.Background(), query()); err != nil {
		t.Fatalf("caller mutation changed cached capabilities: %v", err)
	}
	changed.Store(true)
	if _, err := client.Embed(context.Background(), query()); !errors.Is(err, ErrContract) {
		t.Fatalf("mid-request identity drift accepted: %v", err)
	}
	updated, err := client.Spaces(context.Background(), healthy.URL)
	if err != nil || updated.Spaces[0].Fingerprint == testFingerprint {
		t.Fatalf("refresh did not expose new metadata to registry: %+v, %v", updated, err)
	}
	if _, err := client.Embed(context.Background(), query()); err != nil {
		t.Fatalf("response does not match refreshed metadata: %v", err)
	}
}

func TestEmbedRejectsContractErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*EmbedResponse)
		raw    string
	}{
		{"space", func(r *EmbedResponse) { r.Space = "another-space" }, ""},
		{"fingerprint", func(r *EmbedResponse) { r.Fingerprint = "sha256:" + strings.Repeat("b", 64) }, ""},
		{"declared dimension", func(r *EmbedResponse) { r.Dim = 2 }, ""},
		{"actual dimension", func(r *EmbedResponse) { r.Vectors[0] = []float32{1, 0} }, ""},
		{"count", func(r *EmbedResponse) { r.Vectors = [][]float32{} }, ""},
		{"zero norm", func(r *EmbedResponse) { r.Vectors[0] = []float32{0, 0, 0} }, ""},
		{"not normalized", func(r *EmbedResponse) { r.Vectors[0] = []float32{1, 1, 1} }, ""},
		{"overflow", nil, `{"space":"test-space@r1","fingerprint":"x","dim":3,"vectors":[[1e9999,0,0]]}`},
		{"unknown fields", nil, `{"unexpected":"value"}`},
		{"trailing JSON", nil, `{ } { }`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/spaces" {
					writeJSON(w, testMetadata())
					return
				}
				if tt.raw != "" {
					fmt.Fprint(w, tt.raw)
					return
				}
				response := embedding()
				tt.change(&response)
				writeJSON(w, response)
			}))
			defer server.Close()
			_, err := testClient(server.URL).Embed(context.Background(), query())
			var classified *Error
			if !errors.Is(err, ErrContract) || !errors.As(err, &classified) || classified.Retryable {
				t.Fatalf("contract mismatch not classified as permanent: %v", err)
			}
		})
	}
	for _, value := range []float32{float32(math.NaN()), float32(math.Inf(1))} {
		response := embedding()
		response.Vectors[0][0] = value
		if err := validateVectors(query(), response, testMetadata().Spaces[0]); !errors.Is(err, ErrContract) {
			t.Fatalf("nonfinite vector accepted: %v", err)
		}
	}
}

func TestEmbedInputLimitsAndTextOnlyQuery(t *testing.T) {
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
		}
		metadata := testMetadata()
		metadata.Spaces[0].QueryInputs = []string{"text", "image"}
		writeJSON(w, metadata)
	}))
	defer server.Close()
	client := testClient(server.URL)
	for _, req := range []EmbedRequest{
		{Space: testSpace, Role: "query"},
		{Space: testSpace, Role: "bad", Inputs: query().Inputs},
		{Space: testSpace, Role: "query", Inputs: []EmbedInput{{Image: &ImageRef{URI: "local://media/image.jpg"}}}},
		{Space: testSpace, Role: "document", Inputs: []EmbedInput{{Text: "text", Image: &ImageRef{URI: "local://media/image.jpg"}}}},
		{Space: testSpace, Role: "document", Inputs: []EmbedInput{{Video: &MediaRef{URI: "local://media/video.mp4", EndMs: 2501}}}},
		{Space: testSpace, Role: "document", Inputs: []EmbedInput{{Video: &MediaRef{URI: "local://media/video.mp4", StartMs: 1, EndMs: 1}}}},
		{Space: testSpace, Role: "document", Inputs: []EmbedInput{{Image: &ImageRef{URI: "https://example.com/image.jpg?token=secret"}}}},
		{Space: testSpace, Role: "query", Inputs: slices.Repeat(query().Inputs, 4)},
	} {
		if _, err := client.Embed(context.Background(), req); apperr.KindOf(err) != apperr.Invalid {
			t.Fatalf("invalid request accepted or misclassified: %+v, %v", req, err)
		}
	}
	if posts.Load() != 0 {
		t.Fatalf("invalid inputs reached ML service: %d", posts.Load())
	}
}

func TestTranscribeAbsoluteTimeContract(t *testing.T) {
	for _, tt := range []struct {
		name      string
		change    func(*TranscribeResponse)
		wantError bool
	}{
		{"valid clipped window", func(*TranscribeResponse) {}, false},
		{"silence", func(r *TranscribeResponse) { r.Sentences = []Sentence{} }, false},
		{"fingerprint", func(r *TranscribeResponse) { r.Fingerprint = "changed" }, true},
		{"model", func(r *TranscribeResponse) { r.Model = "different" }, true},
		{"relative timestamp", func(r *TranscribeResponse) { r.Sentences[0].StartMs = 0 }, true},
		{"past clipped end", func(r *TranscribeResponse) { r.Sentences[1].EndMs = 2600 }, true},
		{"ordered sentence overlap", func(r *TranscribeResponse) { r.Sentences[1].StartMs = 1400 }, false},
		{"equal sentence starts and contained end", func(r *TranscribeResponse) {
			r.Sentences[1].StartMs, r.Sentences[1].EndMs = 1000, 1300
		}, false},
		{"backwards sentences", func(r *TranscribeResponse) {
			r.Sentences[0], r.Sentences[1] = r.Sentences[1], r.Sentences[0]
		}, true},
		{"backwards words", func(r *TranscribeResponse) {
			r.Sentences[0].Words[0], r.Sentences[0].Words[1] = r.Sentences[0].Words[1], r.Sentences[0].Words[0]
		}, true},
		{"backwards words across sentences", func(r *TranscribeResponse) {
			r.Sentences[0].EndMs = 2400
			r.Sentences[0].Words = []Word{{W: "先", S: 2200, E: 2400}}
			r.Sentences[1].Words = []Word{{W: "后", S: 2100, E: 2300}}
		}, true},
		{"zero word duration", func(r *TranscribeResponse) { r.Sentences[0].Words[0].E = 1000 }, true},
		{"zero sentence duration", func(r *TranscribeResponse) { r.Sentences[0].EndMs = 1000 }, true},
		{"missing words", func(r *TranscribeResponse) { r.Sentences[0].Words = nil }, true},
		{"word outside sentence", func(r *TranscribeResponse) { r.Sentences[0].Words[0].S = 999 }, true},
		{"missing sentences", func(r *TranscribeResponse) { r.Sentences = nil }, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/spaces" {
					writeJSON(w, testMetadata())
					return
				}
				var req TranscribeRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req != transcription() {
					t.Errorf("request: %+v, %v", req, err)
				}
				response := transcript()
				tt.change(&response)
				writeJSON(w, response)
			}))
			defer server.Close()
			_, err := testClient(server.URL).Transcribe(context.Background(), server.URL, transcription())
			if tt.wantError != errors.Is(err, ErrContract) || (!tt.wantError && err != nil) {
				t.Fatalf("Transcribe: %v, contract error wanted=%v", err, tt.wantError)
			}
		})
	}
}

func TestTranscriptPreservesOrderedWordOverlap(t *testing.T) {
	for _, sentences := range [][]Sentence{
		{{StartMs: 1000, EndMs: 1900, Text: "今天下雨", Words: []Word{{W: "今天", S: 1000, E: 1500}, {W: "下雨", S: 1450, E: 1900}}}},
		{{StartMs: 76830, EndMs: 77460, Text: "warmly", Words: []Word{{W: "warmly", S: 76830, E: 77460}}},
			{StartMs: 77400, EndMs: 77460, Text: "and", Words: []Word{{W: "and", S: 77400, E: 77460}}}},
		{{StartMs: 1000, EndMs: 1900, Text: "包含", Words: []Word{{W: "包", S: 1000, E: 1900}, {W: "含", S: 1000, E: 1500}}}},
	} {
		response := TranscribeResponse{Model: testModel, Fingerprint: testFingerprint, Language: "zh", DurationMs: 78000, Sentences: sentences}
		before, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateTranscript(TranscribeRequest{StartMs: 1000, EndMs: 78000}, response, testMetadata().Transcribers[0]); err != nil {
			t.Fatalf("ordered overlap rejected: %+v: %v", sentences, err)
		}
		after, err := json.Marshal(response)
		if err != nil || string(after) != string(before) {
			t.Fatalf("validation changed original evidence: %s => %s: %v", before, after, err)
		}
	}
}

func TestHTTPErrorClassificationAnd503Retry(t *testing.T) {
	for _, tt := range []struct {
		status    int
		code      string
		retryable bool
		kind      apperr.Kind
		attempts  int
	}{
		{400, "invalid_input", false, apperr.Invalid, 1}, {401, "unauthorized", false, apperr.Internal, 1},
		{404, "unknown_space", false, apperr.NotFound, 1}, {422, "media_unreadable", false, apperr.Invalid, 1},
		{429, "http_error", true, apperr.Unavailable, 1},
		{500, "internal", true, apperr.Unavailable, 1}, {502, "http_error", true, apperr.Unavailable, 1},
		{503, "busy", true, apperr.Unavailable, 4}, {504, "deadline_exceeded", true, apperr.Unavailable, 1},
	} {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			var count atomic.Int32
			var mu sync.Mutex
			var ids []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count.Add(1)
				mu.Lock()
				ids = append(ids, r.Header.Get("X-Request-ID"))
				mu.Unlock()
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(tt.status)
				fmt.Fprintf(w, `{"error":{"code":%q,"message":"private-token and private text"}}`, tt.code)
			}))
			defer server.Close()
			_, err := testClient(server.URL).Spaces(context.Background(), server.URL)
			var classified *Error
			if !errors.As(err, &classified) || classified.Status != tt.status || classified.Code != tt.code || classified.Retryable != tt.retryable || apperr.KindOf(err) != tt.kind || int(count.Load()) != tt.attempts {
				t.Fatalf("classification: %+v, calls=%d", classified, count.Load())
			}
			if strings.Contains(err.Error(), "private") || ((tt.status == 503 || tt.status == 504) && !errors.Is(err, ErrThrottled)) {
				t.Fatalf("unsafe or incorrect error: %v", err)
			}
			mu.Lock()
			defer mu.Unlock()
			for _, id := range ids {
				if id == "" || id != ids[0] {
					t.Fatalf("request ID changed across retries: %v", ids)
				}
			}
		})
	}
}

func TestCancellationAndRetryDeadline(t *testing.T) {
	t.Run("backoff obeys deadline", func(t *testing.T) {
		var count atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count.Add(1)
			w.Header().Set("Retry-After", "10")
			w.WriteHeader(503)
		}))
		defer server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := testClient(server.URL).Spaces(ctx, server.URL)
		if !errors.Is(err, context.DeadlineExceeded) || count.Load() != 1 || time.Since(start) > time.Second {
			t.Fatalf("backoff ignored deadline: %v, calls=%d", err, count.Load())
		}
	})
	t.Run("running request cancels", func(t *testing.T) {
		started := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done() }))
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		result := make(chan error, 1)
		go func() { _, err := testClient(server.URL).Spaces(ctx, server.URL); result <- err }()
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("request did not start")
		}
		cancel()
		select {
		case err := <-result:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("wrong cancellation error: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("request did not cancel")
		}
	})
}

func TestRedirectDoesNotForwardToken(t *testing.T) {
	var contacted atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { contacted.Store(true) }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	if _, err := testClient(source.URL).Spaces(context.Background(), source.URL); err == nil || contacted.Load() {
		t.Fatalf("redirect forwarded internal token: %v", err)
	}
}

func TestDiscoveryRejectsInvalidMetadata(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(*SpacesResponse)
	}{
		{"missing arrays", func(r *SpacesResponse) { r.Spaces = nil }},
		{"duplicate IDs", func(r *SpacesResponse) { r.Spaces = append(r.Spaces, r.Spaces[0]) }},
		{"invalid dimension", func(r *SpacesResponse) { r.Spaces[0].Dim = 0 }},
		{"invalid metric", func(r *SpacesResponse) { r.Spaces[0].Metric = "guessed" }},
		{"invalid fingerprint", func(r *SpacesResponse) { r.Spaces[0].Fingerprint = "unversioned" }},
		{"invalid capability", func(r *SpacesResponse) { r.Spaces[0].DocInputs = []string{"unknown"} }},
		{"invalid batch", func(r *SpacesResponse) { r.Spaces[0].MaxBatch = 0 }},
		{"invalid ASR window", func(r *SpacesResponse) { r.Transcribers[0].MaxWindowMs = 0 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				metadata := testMetadata()
				tt.change(&metadata)
				writeJSON(w, metadata)
			}))
			defer server.Close()
			if _, err := testClient(server.URL).Spaces(context.Background(), server.URL); !errors.Is(err, ErrContract) {
				t.Fatalf("invalid metadata accepted: %v", err)
			}
		})
	}
}
