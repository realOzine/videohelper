package stages

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/pipeline"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/platform/queue"
	"github.com/ozine/videohelper/internal/space"
)

func TestASRAndEmbeddingCheckpointsIntegration(t *testing.T) {
	ctx, pool, objects := integration(t)
	spaceFingerprint := "sha256:" + strings.Repeat("a", 64)
	asrFingerprint := "sha256:" + strings.Repeat("b", 64)
	var mu sync.Mutex
	asrCalls := map[int]int{}
	embedCalls := 0
	failASR := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/spaces":
			span := 30000
			json.NewEncoder(w).Encode(mlclient.SpacesResponse{Spaces: []mlclient.SpaceInfo{{ID: "wemm-2b@r1", Dim: 2, Metric: "cosine", QueryInputs: []string{"text"}, DocInputs: []string{"video", "text", "image"}, MaxSpanMs: &span, MaxBatch: 2, Fingerprint: spaceFingerprint}}, Transcribers: []mlclient.TranscriberInfo{{ID: "paraformer-zh@r1", Languages: []string{"zh"}, MaxWindowMs: 6000, Fingerprint: asrFingerprint}}})
		case "/v1/transcribe":
			var request mlclient.TranscribeRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			mu.Lock()
			asrCalls[request.StartMs]++
			shouldFail := failASR && request.StartMs == 1500
			mu.Unlock()
			if shouldFail {
				w.WriteHeader(500)
				w.Write([]byte(`{"error":{"code":"internal","message":"retry"}}`))
				return
			}
			sentences := []mlclient.Sentence{{StartMs: 1700, EndMs: 2300, Text: "跨窗口的中文句子。", Words: []mlclient.Word{{W: "原文", S: 1700, E: 2300}}}}
			if request.StartMs == 1500 {
				sentences = append(sentences, mlclient.Sentence{StartMs: 2500, EndMs: 3200, Text: "保留原句。", Words: []mlclient.Word{{W: "原句", S: 2500, E: 3200}}})
			}
			json.NewEncoder(w).Encode(mlclient.TranscribeResponse{Model: request.Model, Fingerprint: asrFingerprint, Language: "zh", DurationMs: 4000, Sentences: sentences})
		case "/v1/embed":
			var request mlclient.EmbedRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			mu.Lock()
			embedCalls++
			mu.Unlock()
			vectors := make([][]float32, len(request.Inputs))
			for i := range vectors {
				vectors[i] = []float32{1, 0}
			}
			json.NewEncoder(w).Encode(mlclient.EmbedResponse{Space: request.Space, Fingerprint: spaceFingerprint, Dim: 2, Vectors: vectors})
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	var registry *space.Registry
	mlConfig := config.MLConfig{Endpoints: []string{server.URL}, Token: "fixture", QueryTimeout: time.Second, EmbedTimeout: time.Second, TranscribeTimeout: time.Second, EmbedBatch: 2, TranscribeChunkMS: 2000, TranscribeOverlapMS: 500}
	client := mlclient.New(mlConfig, func(id string) (string, bool) { return registry.Resolve(id) })
	registry = space.New(pool, client)
	if err := registry.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	var workspaceID, videoID int64
	if err := pool.QueryRow(ctx, "INSERT INTO workspace(name) VALUES('encoding') RETURNING id").Scan(&workspaceID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO video(workspace_id,title,source_kind,status,duration_ms,has_video,has_audio,normalized_video_key) VALUES($1,'fixture','upload','indexing',4000,true,true,'normalized_video.mp4') RETURNING id", workspaceID).Scan(&videoID); err != nil {
		t.Fatal(err)
	}
	video, err := library.GetForWorker(ctx, pool, videoID)
	if err != nil {
		t.Fatal(err)
	}
	h := New(pool, objects, client, registry, Config{ML: mlConfig, PresignTTL: time.Minute})
	sc := &pipeline.StageCtx{Video: video, Payload: queue.StagePayload{WorkspaceID: workspaceID, VideoID: videoID, Stage: "asr", Version: "v1.asr"}, Attempt: 1, Def: pipeline.StageDef{Options: pipeline.StageOptions{Transcriber: "paraformer-zh@r1", Fingerprint: asrFingerprint, MaxWindowMs: 6000}}}
	checkpoints := 0
	failCheckpoint := false
	sc.SaveProgress = func(ctx context.Context, value any, commit pipeline.Commit) error {
		checkpoints++
		if failCheckpoint && checkpoints == 2 {
			return errors.New("interrupted before checkpoint transaction")
		}
		if commit != nil {
			if err := db.WithTx(ctx, pool, func(tx pgx.Tx) error { return commit(ctx, tx) }); err != nil {
				return err
			}
		}
		encoded, err := json.Marshal(value)
		if err == nil {
			sc.Progress = encoded
		}
		return err
	}
	if _, err := h.ASR(ctx, sc); err == nil {
		t.Fatal("injected ASR failure ignored")
	}
	if checkpoints != 1 {
		t.Fatalf("ASR checkpoints=%d", checkpoints)
	}
	mu.Lock()
	failASR = false
	mu.Unlock()
	h.cfg.ML.TranscribeChunkMS = 1000 // A resumed attempt keeps the persisted window plan.
	sc.Attempt = 2
	commitRun(t, ctx, pool, sc, h.ASR)
	mu.Lock()
	first, second := asrCalls[0], asrCalls[1500]
	mu.Unlock()
	if first != 1 || second != 2 {
		t.Fatalf("ASR resumed wrong windows: first=%d second=%d", first, second)
	}
	sentences, err := index.SpeechSentences(ctx, pool, videoID)
	if err != nil || len(sentences) != 2 || sentences[0].Text != "跨窗口的中文句子。" || sentences[1].Text != "保留原句。" || sentences[0].Words[0].W != "原文" {
		t.Fatalf("ASR lost or duplicated original evidence: %+v %v", sentences, err)
	}
	info, _ := registry.Get("wemm-2b@r1")
	sc.Def.Options = pipeline.StageOptions{Channel: "visual", Seg: "grid@v1", Space: info}
	sc.Payload.Stage = "embed:visual:wemm-2b@r1"
	sc.Payload.Version = "v1.embed"
	sc.Progress = nil
	checkpoints = 0
	failCheckpoint = true
	if _, err := h.Embed(ctx, sc); err == nil {
		t.Fatal("injected checkpoint failure ignored")
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM embedding WHERE video_id=$1 AND channel='visual'", videoID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("partial batch count=%d %v", count, err)
	}
	failCheckpoint = false
	sc.Attempt++
	if commit, err := h.Embed(ctx, sc); err != nil || commit != nil {
		t.Fatalf("resume=%v %v", commit, err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM embedding WHERE video_id=$1 AND channel='visual'", videoID).Scan(&count); err != nil || count != 4 {
		t.Fatalf("resumed batch count=%d %v", count, err)
	}
	mu.Lock()
	calls := embedCalls
	mu.Unlock()
	if calls != 3 {
		t.Fatalf("already committed batches re-encoded: %d", calls)
	}
	sc.Def.Options.Channel = "speech"
	sc.Def.Options.Seg = "sentence@v1"
	sc.Progress = nil
	if _, err := h.Embed(ctx, sc); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM embedding WHERE video_id=$1 AND channel='speech'", videoID).Scan(&count); err != nil || count == 0 {
		t.Fatalf("speech not embedded: count=%d err=%v", count, err)
	}
	sc.Def.Options.Space.Fingerprint = "sha256:" + strings.Repeat("c", 64)
	if _, err := h.Embed(ctx, sc); err == nil {
		t.Fatal("changed model plan was accepted")
	}
}

func TestModelAbsenceAndIdentityDriftIntegration(t *testing.T) {
	ctx, pool, objects := integration(t)
	fingerprint := "sha256:" + strings.Repeat("a", 64)
	span := 30000
	metadata := mlclient.SpacesResponse{
		Spaces:       []mlclient.SpaceInfo{{ID: "wemm-2b@r1", Dim: 2, Metric: "cosine", QueryInputs: []string{"text"}, DocInputs: []string{"video", "text"}, MaxSpanMs: &span, MaxBatch: 2, Fingerprint: fingerprint}},
		Transcribers: []mlclient.TranscriberInfo{{ID: "asr@r1", Languages: []string{"zh"}, MaxWindowMs: 6000, Fingerprint: fingerprint}},
	}
	var mode atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mode.Load() == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		current := metadata
		if mode.Load() == 2 {
			current.Spaces = append([]mlclient.SpaceInfo(nil), metadata.Spaces...)
			current.Transcribers = append([]mlclient.TranscriberInfo(nil), metadata.Transcribers...)
			current.Spaces[0].Fingerprint = "sha256:" + strings.Repeat("b", 64)
			current.Transcribers[0].Fingerprint = current.Spaces[0].Fingerprint
		}
		json.NewEncoder(w).Encode(current)
	}))
	defer server.Close()
	client := mlclient.New(config.MLConfig{Endpoints: []string{server.URL}, Token: "fixture"}, nil)
	registry := space.New(pool, client)
	if err := registry.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	info, _ := registry.Get(metadata.Spaces[0].ID)
	handlers := New(pool, objects, client, registry, Config{})
	sc := &pipeline.StageCtx{Def: pipeline.StageDef{Options: pipeline.StageOptions{Space: info, Transcriber: "asr@r1", Fingerprint: fingerprint}}}
	mode.Store(1)
	if err := registry.Sync(ctx); err == nil {
		t.Fatal("endpoint outage was not reported")
	}
	for name, run := range map[string]pipeline.RunFunc{"asr": handlers.ASR, "embed": handlers.Embed} {
		if _, err := run(ctx, sc); apperr.KindOf(err) != apperr.Unavailable {
			t.Fatalf("%s missing route became permanent: %v", name, err)
		}
	}
	mode.Store(2)
	if err := registry.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	for name, run := range map[string]pipeline.RunFunc{"asr": handlers.ASR, "embed": handlers.Embed} {
		if _, err := run(ctx, sc); apperr.KindOf(err) != apperr.Invalid {
			t.Fatalf("%s frozen model drift remained retryable: %v", name, err)
		}
	}
}
