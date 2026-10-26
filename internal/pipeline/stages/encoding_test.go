package stages

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	var speechInputs []mlclient.EmbedInput
	failASR := true
	var asrMode atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/spaces":
			span := 30000
			models := []mlclient.TranscriberInfo{{ID: "replacement-asr@r1", Languages: []string{"zh"}, MaxWindowMs: 6000, Fingerprint: "sha256:" + strings.Repeat("c", 64)}}
			if asrMode.Load() != 1 {
				fingerprint := asrFingerprint
				if asrMode.Load() == 2 {
					fingerprint = models[0].Fingerprint
				}
				models = append(models, mlclient.TranscriberInfo{ID: "paraformer-zh@r1", Languages: []string{"zh"}, MaxWindowMs: 6000, Fingerprint: fingerprint})
			}
			json.NewEncoder(w).Encode(mlclient.SpacesResponse{Spaces: []mlclient.SpaceInfo{{ID: "wemm-2b@r1", Dim: 2, Metric: "cosine", QueryInputs: []string{"text"}, DocInputs: []string{"video", "text", "image"}, MaxSpanMs: &span, MaxBatch: 2, Fingerprint: spaceFingerprint}}, Transcribers: models})
		case "/v1/transcribe":
			var request mlclient.TranscribeRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			if request.Model != "paraformer-zh@r1" {
				t.Errorf("frozen plan called replacement ASR: %s", request.Model)
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
				sentences = append(sentences, mlclient.Sentence{StartMs: 2100, EndMs: 3200, Text: "保留原句。", Words: []mlclient.Word{{W: "原句", S: 2500, E: 3200}}})
			} else {
				sentences[0].Words = append(sentences[0].Words, mlclient.Word{W: "尾词", S: 2200, E: 2300})
				sentences = append([]mlclient.Sentence{{StartMs: 1000, EndMs: 1900, Text: "今天下雨", Words: []mlclient.Word{{W: "今天", S: 1000, E: 1500}, {W: "下雨", S: 1450, E: 1900}}}}, sentences...)
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
			if len(request.Inputs) > 0 && request.Inputs[0].Text != "" {
				speechInputs = append(speechInputs, request.Inputs...)
			}
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
	mlConfig := config.MLConfig{Transcriber: "replacement-asr@r1", Endpoints: []string{server.URL}, Token: "fixture", QueryTimeout: time.Second, EmbedTimeout: time.Second, TranscribeTimeout: time.Second, EmbedBatch: 2, TranscribeChunkMS: 2000, TranscribeOverlapMS: 500}
	client := mlclient.New(mlConfig, func(id string) (string, bool) { return registry.Resolve(id) })
	registry = space.New(pool, client, mlConfig)
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
	newPlan, err := pipeline.ExpandPlan(video, nil, registry)
	if err != nil || newPlan[1].Options.Transcriber != "replacement-asr@r1" || newPlan[1].Options.Fingerprint == asrFingerprint {
		t.Fatalf("new plan did not freeze configured replacement: %+v %v", newPlan, err)
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
	if err != nil || len(sentences) != 3 || sentences[0].Text != "今天下雨" || sentences[1].Text != "跨窗口的中文句子。" || sentences[2].Text != "保留原句。" ||
		!reflect.DeepEqual(sentences[0].Words, []index.Word{{W: "今天", S: 1000, E: 1500}, {W: "下雨", S: 1450, E: 1900}}) || sentences[1].Span.EndMs <= sentences[2].Span.StartMs {
		t.Fatalf("ASR lost or duplicated original evidence: %+v %v", sentences, err)
	}
	var progress asrProgress
	if err := json.Unmarshal(sc.Progress, &progress); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, code string
		chunk      int
		change     func(*mlclient.TranscribeResponse)
	}{
		{"backwards checkpoint words", "", 0, func(r *mlclient.TranscribeResponse) {
			r.Sentences[0].Words[0], r.Sentences[0].Words[1] = r.Sentences[0].Words[1], r.Sentences[0].Words[0]
		}},
		{"checkpoint beyond window", "", 0, func(r *mlclient.TranscribeResponse) { r.Sentences[0].EndMs = 2600 }},
		{"backwards cross-chunk words", "asr_overlap", 1, func(r *mlclient.TranscribeResponse) { r.Sentences[1].Words[0].S = 2100 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var response mlclient.TranscribeResponse
			key := progress.Keys[tt.chunk]
			if err := h.readTranscript(ctx, key, &response); err != nil {
				t.Fatal(err)
			}
			original, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			tt.change(&response)
			changed, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			if err := objects.Put(ctx, key, bytes.NewReader(changed), "application/json"); err != nil {
				t.Fatal(err)
			}
			commit, err := h.ASR(ctx, sc)
			var classified *apperr.Error
			if commit != nil || (tt.code == "" && !errors.Is(err, mlclient.ErrContract)) ||
				(tt.code != "" && (!errors.As(err, &classified) || classified.Code != tt.code)) {
				t.Errorf("unsafe checkpoint replay accepted: commit=%v err=%v", commit, err)
			}
			if err := objects.Put(ctx, key, bytes.NewReader(original), "application/json"); err != nil {
				t.Fatal(err)
			}
		})
	}
	checkpoint := append([]byte(nil), sc.Progress...)
	for _, mode := range []int32{1, 2} {
		asrMode.Store(mode)
		if err := registry.Sync(ctx); err != nil {
			t.Fatal(err)
		}
		_, err := h.ASR(ctx, sc)
		want := apperr.Unavailable
		if mode == 2 {
			want = apperr.Invalid
		}
		if apperr.KindOf(err) != want || !bytes.Equal(sc.Progress, checkpoint) {
			t.Fatalf("frozen ASR absence/drift accepted or rewrote checkpoint: mode=%d err=%v", mode, err)
		}
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
	mu.Lock()
	gotSpeech := append([]mlclient.EmbedInput(nil), speechInputs...)
	mu.Unlock()
	if !reflect.DeepEqual(gotSpeech, []mlclient.EmbedInput{{Text: "今天下雨跨窗口的中文句子。保留原句。"}}) {
		t.Fatalf("speech embedding lost original text order: %+v", gotSpeech)
	}
	var start, end int
	if err := pool.QueryRow(ctx, "SELECT start_ms,end_ms FROM embedding WHERE video_id=$1 AND channel='speech'", videoID).Scan(&start, &end); err != nil || start != 1000 || end != 3200 {
		t.Fatalf("speech union lost source interval: [%d,%d) %v", start, end, err)
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
	registry := space.New(pool, client, config.MLConfig{Transcriber: "asr@r1"})
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
