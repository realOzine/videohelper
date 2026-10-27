package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	desktopruntime "github.com/ozine/videohelper/internal/desktop/runtime"
	"github.com/ozine/videohelper/internal/desktop/store"
	"github.com/ozine/videohelper/internal/mlclient"
)

type fakeML struct {
	mu               sync.Mutex
	inputs           []mlclient.EmbedRequest
	transcripts      []mlclient.TranscribeRequest
	failEmbed        bool
	block            chan struct{}
	entered          chan struct{}
	fingerprint      string
	customTranscript func(mlclient.TranscribeRequest) mlclient.TranscribeResponse
}

func newFakeML(t *testing.T) (*fakeML, *httptest.Server) {
	t.Helper()
	f := &fakeML{fingerprint: "sha256:" + strings.Repeat("a", 64)}
	server := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(server.Close)
	return f, server
}

func (f *fakeML) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	f.mu.Lock()
	fp := f.fingerprint
	f.mu.Unlock()
	switch r.URL.Path {
	case "/v1/spaces":
		maxSpan := 30000
		json.NewEncoder(w).Encode(mlclient.SpacesResponse{Spaces: []mlclient.SpaceInfo{{ID: "visual@r1", Fingerprint: fp, Dim: 2, Metric: "cosine", MaxBatch: 4, MaxSpanMs: &maxSpan, DocInputs: []string{"video", "text"}, QueryInputs: []string{"text"}}, {ID: "audio@r1", Fingerprint: fp, Dim: 2, Metric: "cosine", MaxBatch: 4, MaxSpanMs: &maxSpan, DocInputs: []string{"audio"}, QueryInputs: []string{"text"}}}, Transcribers: []mlclient.TranscriberInfo{{ID: "asr@r1", Fingerprint: fp, Languages: []string{"zh"}, MaxWindowMs: 70000}}})
	case "/v1/embed":
		var req mlclient.EmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		f.mu.Lock()
		f.inputs = append(f.inputs, req)
		fail := f.failEmbed && req.Role == "document"
		f.failEmbed = false
		f.mu.Unlock()
		if fail {
			w.WriteHeader(500)
			fmt.Fprint(w, `{"error":{"code":"internal","message":"test failure"}}`)
			return
		}
		vecs := make([][]float32, len(req.Inputs))
		for i := range vecs {
			vecs[i] = []float32{1, 0}
		}
		json.NewEncoder(w).Encode(mlclient.EmbedResponse{Space: req.Space, Fingerprint: fp, Dim: 2, Vectors: vecs})
	case "/v1/transcribe":
		var req mlclient.TranscribeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		f.mu.Lock()
		f.transcripts = append(f.transcripts, req)
		block, entered, custom := f.block, f.entered, f.customTranscript
		f.mu.Unlock()
		if entered != nil {
			select {
			case entered <- struct{}{}:
			default:
			}
		}
		if block != nil {
			select {
			case <-r.Context().Done():
				return
			case <-block:
			}
		}
		if custom != nil {
			json.NewEncoder(w).Encode(custom(req))
			return
		}
		json.NewEncoder(w).Encode(mlclient.TranscribeResponse{Model: req.Model, Fingerprint: fp, Language: "zh", DurationMs: 1000, Sentences: []mlclient.Sentence{{StartMs: 0, EndMs: 600, Text: "保留原始中文。", Words: []mlclient.Word{{W: "保留", S: 0, E: 500}}}}})
	default:
		http.NotFound(w, r)
	}
}

func TestASRAdjacentWindowsPreserveOverlappingSentences(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	f, server := newFakeML(t)
	p, err := New(ctx, s, Config{Endpoints: []string{server.URL}, Token: "test", VisualSpace: "visual@r1", AudioSpace: "audio@r1", Transcriber: "asr@r1"})
	if err != nil {
		t.Fatal(err)
	}
	f.customTranscript = func(req mlclient.TranscribeRequest) mlclient.TranscribeResponse {
		sentence := mlclient.Sentence{StartMs: 58000, EndMs: 64000, Text: "边界前半句原文。", Words: []mlclient.Word{{W: "前", S: 58000, E: 60000}, {W: "半句", S: 63200, E: 64000}}}
		if req.StartMs > 0 {
			sentence = mlclient.Sentence{StartMs: 62000, EndMs: 66000, Text: "边界后半句原文。", Words: []mlclient.Word{{W: "后", S: 62000, E: 64000}, {W: "半句", S: 65000, E: 66000}}}
		}
		return mlclient.TranscribeResponse{Model: req.Model, Fingerprint: f.fingerprint, Language: "zh", DurationMs: 120000, Sentences: []mlclient.Sentence{sentence}}
	}
	source := filepath.Join(t.TempDir(), "source.mp4")
	if err := os.WriteFile(source, []byte("protocol regression fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	v, err := p.ImportFile(ctx, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	duration := 120000
	v.DurationMs = &duration
	cp := checkpoint{Identity: p.identity, Done: map[string]bool{}, Next: map[string]int{}, Stages: map[string]StageStatus{}}
	if err := p.transcribe(ctx, j, &cp, v); err != nil {
		t.Fatal(err)
	}
	sentences, err := s.Sentences(ctx, v.ID, nil)
	if err != nil || len(sentences) != 2 || sentences[0].Text != "边界前半句原文。" || sentences[1].Text != "边界后半句原文。" {
		t.Fatalf("window overlap lost original text: %+v %v", sentences, err)
	}
	if len(cp.RawKeys) != 2 || !cp.Done["asr"] {
		t.Fatalf("missing replay evidence: %+v", cp)
	}
	f.mu.Lock()
	f.customTranscript = func(req mlclient.TranscribeRequest) mlclient.TranscribeResponse {
		return mlclient.TranscribeResponse{Model: req.Model, Fingerprint: f.fingerprint, Language: "zh", DurationMs: 120000, Sentences: []mlclient.Sentence{{StartMs: 58000, EndMs: 64000, Text: "单窗口乱序。", Words: []mlclient.Word{{W: "后", S: 63200, E: 64000}, {W: "前", S: 58000, E: 60000}}}}}
	}
	f.mu.Unlock()
	invalid, err := p.ImportFile(ctx, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	invalid.DurationMs = &duration
	invalidJob, err := s.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	invalidCP := checkpoint{Identity: p.identity, Done: map[string]bool{}, Next: map[string]int{}, Stages: map[string]StageStatus{}}
	if err := p.transcribe(ctx, invalidJob, &invalidCP, invalid); !errors.Is(err, mlclient.ErrContract) {
		t.Fatalf("single-response disorder was accepted: %v", err)
	}
	if rows, err := s.Sentences(ctx, invalid.ID, nil); err != nil || len(rows) != 0 {
		t.Fatalf("invalid transcript was published: %+v %v", rows, err)
	}
}

func processorFixture(t *testing.T) (*Processor, *store.Store, *fakeML, string) {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is needed for the real media fixture")
	}
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is needed for the real media fixture")
	}
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	f, server := newFakeML(t)
	p, err := New(ctx, s, Config{Tools: desktopruntime.Tools{FFmpeg: ffmpeg, FFprobe: ffprobe}, Endpoints: []string{server.URL}, Token: "test-private-ml-token", VisualSpace: "visual@r1", AudioSpace: "audio@r1", Transcriber: "asr@r1"})
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "original.mp4")
	var stderr bytes.Buffer
	err = desktopruntime.RunCommand(ctx, ffmpeg, []string{"-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "color=c=blue:s=160x90:r=10:d=1", "-f", "lavfi", "-i", "sine=frequency=400:sample_rate=16000:duration=1", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", source}, nil, &stderr)
	if err != nil {
		t.Fatalf("fixture ffmpeg %v: %s", err, stderr.String())
	}
	return p, s, f, source
}

func TestProcessorMediaCheckpointsSearchAndIdentity(t *testing.T) {
	p, s, f, source := processorFixture(t)
	ctx := context.Background()
	v, err := p.ImportFile(ctx, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// The first embedding fails after ASR, exercising actual checkpoint resume.
	f.mu.Lock()
	f.failEmbed = true
	f.mu.Unlock()
	err = p.process(ctx, j)
	if err == nil {
		t.Fatal("expected model failure")
	}
	if err := s.FailJob(ctx, j.ID, j.Attempt, err.Error()); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Retry(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	j, err = s.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.process(ctx, j); err != nil {
		t.Fatal(err)
	}
	v, err = s.Video(ctx, v.ID)
	if err != nil || v.Status != "ready" {
		t.Fatalf("%+v %v", v, err)
	}
	if v.SourceKey == v.NormalizedVideoKey || v.NormalizedVideoKey == "" || v.Thumbs == nil {
		t.Fatalf("media publication: %+v", v)
	}
	f.mu.Lock()
	requests := append([]mlclient.EmbedRequest{}, f.inputs...)
	transcripts := append([]mlclient.TranscribeRequest{}, f.transcripts...)
	f.mu.Unlock()
	if len(transcripts) != 1 || transcripts[0].URI != "local://media/"+v.SourceKey {
		t.Fatalf("ASR replayed or consumed lossy media: %+v", transcripts)
	}
	var audio, visual bool
	for _, req := range requests {
		for _, input := range req.Inputs {
			if input.Audio != nil {
				audio = true
				if input.Audio.URI != "local://media/"+v.SourceKey {
					t.Fatal("CLAP did not retain source audio")
				}
			}
			if input.Video != nil {
				visual = true
				if input.Video.URI != "local://media/"+v.NormalizedVideoKey {
					t.Fatal("visual did not use normalized video")
				}
			}
		}
	}
	if !audio || !visual {
		t.Fatal("missing audio/video indexing")
	}
	query := "  原样查询  "
	result, err := p.Search(ctx, Query{Text: query})
	if err != nil || len(result.Moments) != 1 || result.Moments[0].VideoID != v.ID || result.Moments[0].Snippet != "保留原始中文。" {
		t.Fatalf("%+v %v", result, err)
	}
	f.mu.Lock()
	last := f.inputs[len(f.inputs)-1]
	f.mu.Unlock()
	if last.Inputs[0].Text != query {
		t.Fatalf("query was rewritten: %q", last.Inputs[0].Text)
	}
	models, err := p.Models(ctx, Query{})
	if err != nil || len(models) != 3 || models[0].Models[0].IndexedVideos != 1 || models[2].DefaultWeight != 0 {
		t.Fatalf("%+v %v", models, err)
	}
	stages, err := p.Stages(ctx, v.ID)
	if err != nil || len(stages) != 7 {
		t.Fatalf("%+v %v", stages, err)
	}
	if err := p.Cancel(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("user source was removed")
	}
	// A hardware/model fingerprint change keeps old rows but reports coverage
	// incomplete for the new identity and schedules a fresh job only on Retry.
	f.mu.Lock()
	f.fingerprint = "sha256:" + strings.Repeat("b", 64)
	f.mu.Unlock()
	p2, err := New(ctx, s, p.cfg)
	if err != nil {
		t.Fatal(err)
	}
	models, err = p2.Models(ctx, Query{})
	if err != nil || models[0].Models[0].Status != "indexing" || models[0].Models[0].IndexedVideos != 0 {
		t.Fatalf("false new-model readiness: %+v %v", models, err)
	}
	spaces, err := s.Spaces(ctx)
	if err != nil || len(spaces) != 6 {
		t.Fatalf("old model spaces were lost: %d %v", len(spaces), err)
	}
	fresh, err := p2.Retry(ctx, v.ID)
	if err != nil || fresh.ID == j.ID {
		t.Fatalf("did not create new model job: %+v %v", fresh, err)
	}
}

func TestProcessorCancelStopsInflightAndPreventsLateReady(t *testing.T) {
	p, s, f, source := processorFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.mu.Lock()
	f.block = make(chan struct{})
	f.entered = make(chan struct{}, 1)
	block, entered := f.block, f.entered
	f.mu.Unlock()
	defer close(block)
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	v, err := p.ImportFile(ctx, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(15 * time.Second):
		t.Fatal("ASR did not start")
	}
	if err := p.Cancel(context.Background(), v.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		j, err := s.LatestJob(context.Background(), v.ID)
		if err != nil {
			t.Fatal(err)
		}
		p.mu.Lock()
		active := len(p.active)
		p.mu.Unlock()
		if j.Status == "cancelled" && active == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("inflight cancellation did not finish: %+v active=%d", j, active)
		}
		time.Sleep(10 * time.Millisecond)
	}
	v, err = s.Video(context.Background(), v.ID)
	if err != nil || v.Status == "ready" {
		t.Fatalf("late completion escaped cancel: %+v %v", v, err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("processor did not stop")
	}
}
