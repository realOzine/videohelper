package mlclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/config"
)

type Resolver func(spaceID string) (baseURL string, ok bool)

var (
	ErrContract  = errors.New("ML contract mismatch")
	ErrThrottled = errors.New("ML throttled")
)

type Error struct {
	Status    int
	Code      string
	Retryable bool
	Err       error
}

func (e *Error) Error() string { return "ML: " + e.Code }
func (e *Error) Unwrap() error { return e.Err }

func failure(kind apperr.Kind, status int, code string, retryable bool, cause error) *Error {
	return &Error{Status: status, Code: code, Retryable: retryable, Err: apperr.Wrap(kind, code, cause)}
}

func contract(reason string) error {
	return failure(apperr.Internal, 0, "contract_mismatch", false, fmt.Errorf("%w: %s", ErrContract, reason))
}

func invalid(reason string) error {
	return failure(apperr.Invalid, 0, "invalid_input", false, errors.New(reason))
}

type contextKey int

const (
	requestIDKey contextKey = iota
	traceparentKey
)

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func WithTraceparent(ctx context.Context, value string) context.Context {
	return context.WithValue(ctx, traceparentKey, value)
}

func callContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if RequestID(ctx) == "" {
		ctx = WithRequestID(ctx, rand.Text())
	}
	return context.WithTimeout(ctx, timeout)
}

type Client struct {
	cfg     config.MLConfig
	resolve Resolver
	http    *http.Client
	mu      sync.RWMutex
	known   map[string]SpacesResponse
}

func New(cfg config.MLConfig, resolve Resolver) *Client {
	if cfg.QueryTimeout <= 0 {
		cfg.QueryTimeout = 5 * time.Second
	}
	if cfg.EmbedTimeout <= 0 {
		cfg.EmbedTimeout = time.Minute
	}
	if cfg.TranscribeTimeout <= 0 {
		cfg.TranscribeTimeout = 10 * time.Minute
	}
	cfg.Endpoints = slices.Clone(cfg.Endpoints)
	return &Client{cfg: cfg, resolve: resolve, known: make(map[string]SpacesResponse), http: &http.Client{
		// Never forward the internal token to a redirected endpoint.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// Discover retains healthy endpoints even if another endpoint is unavailable.
func (c *Client) Discover(ctx context.Context) Discovery {
	out := Discovery{Endpoints: make(map[string]SpacesResponse), Errors: make(map[string]error)}
	for _, endpoint := range c.cfg.Endpoints {
		response, err := c.Spaces(ctx, endpoint)
		if err != nil {
			out.Errors[endpoint] = err
		} else {
			out.Endpoints[endpoint] = response
		}
	}
	return out
}

func (c *Client) Spaces(ctx context.Context, baseURL string) (SpacesResponse, error) {
	ctx, cancel := callContext(ctx, 5*time.Second)
	defer cancel()
	var response SpacesResponse
	if err := c.do(ctx, baseURL, "/v1/spaces", nil, &response); err != nil {
		return response, err
	}
	if err := validateSpaces(response); err != nil {
		return SpacesResponse{}, err
	}
	c.mu.Lock()
	c.known[strings.TrimRight(baseURL, "/")] = cloneSpaces(response)
	c.mu.Unlock()
	return response, nil
}

func cloneSpaces(in SpacesResponse) SpacesResponse {
	out := SpacesResponse{Spaces: slices.Clone(in.Spaces), Transcribers: slices.Clone(in.Transcribers)}
	for i := range out.Spaces {
		out.Spaces[i].QueryInputs = slices.Clone(in.Spaces[i].QueryInputs)
		out.Spaces[i].DocInputs = slices.Clone(in.Spaces[i].DocInputs)
		if in.Spaces[i].MaxSpanMs != nil {
			value := *in.Spaces[i].MaxSpanMs
			out.Spaces[i].MaxSpanMs = &value
		}
	}
	for i := range out.Transcribers {
		out.Transcribers[i].Languages = slices.Clone(in.Transcribers[i].Languages)
	}
	return out
}

func (c *Client) metadata(ctx context.Context, endpoint string) (SpacesResponse, error) {
	c.mu.RLock()
	info, ok := c.known[strings.TrimRight(endpoint, "/")]
	c.mu.RUnlock()
	if ok {
		return info, nil
	}
	if _, err := c.Spaces(ctx, endpoint); err != nil {
		return SpacesResponse{}, err
	}
	c.mu.RLock()
	info = c.known[strings.TrimRight(endpoint, "/")]
	c.mu.RUnlock()
	return info, nil
}

func (c *Client) Embed(ctx context.Context, req EmbedRequest) (EmbedResponse, error) {
	timeout := c.cfg.EmbedTimeout
	if req.Role == "query" {
		timeout = c.cfg.QueryTimeout
	}
	ctx, cancel := callContext(ctx, timeout)
	defer cancel()
	if c.resolve == nil {
		return EmbedResponse{}, failure(apperr.NotFound, 404, "unknown_space", false, nil)
	}
	endpoint, ok := c.resolve(req.Space)
	if !ok {
		return EmbedResponse{}, failure(apperr.NotFound, 404, "unknown_space", false, nil)
	}
	metadata, err := c.metadata(ctx, endpoint)
	if err != nil {
		return EmbedResponse{}, err
	}
	for _, info := range metadata.Spaces {
		if info.ID != req.Space {
			continue
		}
		if err := validateEmbedRequest(req, info); err != nil {
			return EmbedResponse{}, err
		}
		var response EmbedResponse
		if err := c.do(ctx, endpoint, "/v1/embed", req, &response); err != nil {
			return EmbedResponse{}, err
		}
		if err := validateVectors(req, response, info); err != nil {
			return EmbedResponse{}, err
		}
		return response, nil
	}
	return EmbedResponse{}, failure(apperr.NotFound, 404, "unknown_space", false, nil)
}

func (c *Client) Transcribe(ctx context.Context, baseURL string, req TranscribeRequest) (TranscribeResponse, error) {
	ctx, cancel := callContext(ctx, c.cfg.TranscribeTimeout)
	defer cancel()
	metadata, err := c.metadata(ctx, baseURL)
	if err != nil {
		return TranscribeResponse{}, err
	}
	for _, info := range metadata.Transcribers {
		if info.ID != req.Model {
			continue
		}
		if !validMedia(req.URI, req.StartMs, req.EndMs) || req.EndMs-req.StartMs > info.MaxWindowMs || !slices.Contains(info.Languages, req.Language) {
			return TranscribeResponse{}, invalid("invalid transcription window or language")
		}
		var response TranscribeResponse
		if err := c.do(ctx, baseURL, "/v1/transcribe", req, &response); err != nil {
			return TranscribeResponse{}, err
		}
		if err := validateTranscript(req, response, info); err != nil {
			return TranscribeResponse{}, err
		}
		return response, nil
	}
	return TranscribeResponse{}, failure(apperr.NotFound, 404, "unknown_model", false, nil)
}

const maxResponseBytes = 32 << 20

func (c *Client) do(ctx context.Context, baseURL, path string, body, out any) error {
	base, err := url.Parse(baseURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") || base.User != nil || base.RawQuery != "" || base.Fragment != "" {
		return invalid("invalid ML endpoint")
	}
	if c.cfg.Token == "" {
		return failure(apperr.Internal, 401, "unauthorized", false, nil)
	}
	id := RequestID(ctx)
	if len(id) == 0 || len(id) > 256 || strings.ContainsAny(id, "\r\n") {
		return invalid("invalid request ID")
	}
	var payload []byte
	method := http.MethodGet
	if body != nil {
		method = http.MethodPost
		payload, err = json.Marshal(body)
		if err != nil {
			return invalid("cannot encode ML request")
		}
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(payload))
		if err != nil {
			return invalid("invalid ML request")
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Internal-Token", c.cfg.Token)
		req.Header.Set("X-Request-ID", id)
		if trace, _ := ctx.Value(traceparentKey).(string); trace != "" {
			req.Header.Set("traceparent", trace)
		}
		if deadline, ok := ctx.Deadline(); ok {
			remaining := time.Until(deadline).Milliseconds()
			if remaining <= 0 {
				return failure(apperr.Unavailable, 0, "deadline_exceeded", true, context.DeadlineExceeded)
			}
			req.Header.Set("X-Timeout-Ms", strconv.FormatInt(remaining, 10))
		}
		resp, err := c.http.Do(req)
		if err != nil {
			retryable := !errors.Is(err, context.Canceled)
			return failure(apperr.Unavailable, 0, "transport_error", retryable, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
		resp.Body.Close()
		if readErr != nil {
			return failure(apperr.Unavailable, resp.StatusCode, "transport_error", !errors.Is(readErr, context.Canceled), readErr)
		}
		if resp.StatusCode == http.StatusServiceUnavailable && attempt < 3 {
			timer := time.NewTimer(retryAfter(resp.Header.Get("Retry-After")))
			select {
			case <-ctx.Done():
				timer.Stop()
				return failure(apperr.Unavailable, 503, "busy", true, ctx.Err())
			case <-timer.C:
				continue
			}
		}
		if resp.StatusCode != http.StatusOK {
			return responseError(resp.StatusCode, data)
		}
		if len(data) > maxResponseBytes {
			return contract("response exceeds size limit")
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(out); err != nil {
			return contract("invalid JSON response")
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			return contract("trailing JSON response")
		}
		return nil
	}
}

func retryAfter(value string) time.Duration {
	if seconds, err := strconv.ParseInt(value, 10, 32); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if deadline, err := http.ParseTime(value); err == nil {
		return max(time.Until(deadline), 0)
	}
	return 2 * time.Second
}

func responseError(status int, data []byte) error {
	var body struct {
		Error struct{ Code string }
	}
	json.Unmarshal(data, &body)
	code := body.Error.Code
	allowed := map[int][]string{
		400: {"invalid_input"}, 401: {"unauthorized"}, 404: {"unknown_space", "unknown_model"},
		422: {"media_unreadable"}, 500: {"internal"}, 503: {"busy", "not_ready"}, 504: {"deadline_exceeded"},
	}
	if !slices.Contains(allowed[status], code) {
		code = "http_error"
	}
	kind, retryable := apperr.Internal, false
	var cause error
	switch status {
	case 400, 422:
		kind = apperr.Invalid
	case 404:
		kind = apperr.NotFound
	case 408, 429, 500, 502, 503, 504:
		kind, retryable = apperr.Unavailable, true
		if status == 503 || status == 504 {
			cause = ErrThrottled
		}
	}
	return failure(kind, status, code, retryable, cause)
}
