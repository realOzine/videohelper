package stages

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/pipeline"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/platform/queue"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/internal/timeline"
)

// Synthetic CLAP responses test Go batching/checkpoints, not sound-retrieval quality.
func TestAudioEmbeddingResumesBoundedGridBatchesIntegration(t *testing.T) {
	for _, duration := range []int{4500, 90500} {
		t.Run((time.Duration(duration) * time.Millisecond).String(), func(t *testing.T) {
			ctx, pool, objects := integration(t)
			const id = "clap-htsat-unfused@r1"
			fingerprint := "sha256:" + strings.Repeat("a", 64)
			maxSpan := 10000
			var mu sync.Mutex
			var requests [][]mlclient.EmbedInput
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/spaces" {
					json.NewEncoder(w).Encode(mlclient.SpacesResponse{Spaces: []mlclient.SpaceInfo{{ID: id, Dim: 512, Metric: "cosine", QueryInputs: []string{"text"}, DocInputs: []string{"audio"}, MaxSpanMs: &maxSpan, MaxBatch: 2, Fingerprint: fingerprint}}, Transcribers: []mlclient.TranscriberInfo{}})
					return
				}
				var req mlclient.EmbedRequest
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				if req.Space != id || req.Role != "document" || len(req.Inputs) > 2 {
					t.Errorf("wrong audio batch contract: %+v", req)
				}
				for _, input := range req.Inputs {
					if input.Audio == nil || input.Video != nil || input.Image != nil || input.Text != "" || input.Audio.StartMs < 0 || input.Audio.EndMs > duration || input.Audio.EndMs <= input.Audio.StartMs || input.Audio.EndMs-input.Audio.StartMs > maxSpan {
						t.Errorf("invalid audio-only bounded input: %+v", input)
					}
				}
				mu.Lock()
				requests = append(requests, req.Inputs)
				mu.Unlock()
				vectors := make([][]float32, len(req.Inputs))
				for i := range vectors {
					vectors[i] = make([]float32, 512)
					vectors[i][0] = 1
				}
				json.NewEncoder(w).Encode(mlclient.EmbedResponse{Space: id, Fingerprint: fingerprint, Dim: 512, Vectors: vectors})
			}))
			defer server.Close()
			var registry *space.Registry
			cfg := config.MLConfig{Endpoints: []string{server.URL}, Token: "fixture", EmbedTimeout: time.Second, EmbedBatch: 3}
			client := mlclient.New(cfg, func(id string) (string, bool) { return registry.Resolve(id) })
			registry = space.New(pool, client)
			if err := registry.Sync(ctx); err != nil {
				t.Fatal(err)
			}
			var workspaceID, videoID int64
			if err := pool.QueryRow(ctx, "INSERT INTO workspace(name) VALUES('audio-batches') RETURNING id").Scan(&workspaceID); err != nil {
				t.Fatal(err)
			}
			if err := pool.QueryRow(ctx, "INSERT INTO video(workspace_id,title,source_kind,status,duration_ms,has_video,has_audio) VALUES($1,'audio-only','upload','indexing',$2,false,true) RETURNING id", workspaceID, duration).Scan(&videoID); err != nil {
				t.Fatal(err)
			}
			video, err := library.GetForWorker(ctx, pool, videoID)
			if err != nil {
				t.Fatal(err)
			}
			video.NormalizedVideoKey = blob.RunNormalizedVideoKey(workspaceID, videoID, "v1.prepare", 3)
			info, _ := registry.Get(id)
			sc := &pipeline.StageCtx{Video: video, Payload: queue.StagePayload{WorkspaceID: workspaceID, VideoID: videoID, Stage: "embed:audio:" + id, Version: "v1.audio"}, Attempt: 1,
				Def: pipeline.StageDef{Options: pipeline.StageOptions{Channel: "audio", Seg: timeline.GridSeg, Space: info}}}
			checkpoints := 0
			fail := true
			sc.SaveProgress = func(ctx context.Context, value any, commit pipeline.Commit) error {
				checkpoints++
				if fail && checkpoints == 2 {
					return errors.New("interrupted before atomic checkpoint")
				}
				if err := db.WithTx(ctx, pool, func(tx pgx.Tx) error { return commit(ctx, tx) }); err != nil {
					return err
				}
				encoded, err := json.Marshal(value)
				if err == nil {
					sc.Progress = encoded
				}
				return err
			}
			h := New(pool, objects, client, registry, Config{ML: cfg})
			if _, err := h.Embed(ctx, sc); err == nil {
				t.Fatal("checkpoint interruption ignored")
			}
			var count int
			if err := pool.QueryRow(ctx, "SELECT count(*) FROM embedding WHERE video_id=$1 AND channel='audio'", videoID).Scan(&count); err != nil || count != 2 {
				t.Fatalf("uncommitted audio batch leaked: rows=%d error=%v", count, err)
			}
			fail = false
			sc.Attempt++
			if commit, err := h.Embed(ctx, sc); err != nil || commit != nil {
				t.Fatalf("audio resume failed: %v", err)
			}
			rows, err := pool.Query(ctx, "SELECT start_ms,end_ms FROM embedding WHERE video_id=$1 AND channel='audio' ORDER BY start_ms", videoID)
			if err != nil {
				t.Fatal(err)
			}
			var got []timeline.Span
			for rows.Next() {
				var span timeline.Span
				if err := rows.Scan(&span.StartMs, &span.EndMs); err != nil {
					t.Fatal(err)
				}
				got = append(got, span)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				t.Fatal(err)
			}
			want := timeline.Cells(duration)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("audio grid changed across resume: got=%v want=%v", got, want)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(requests) != (len(want)+1)/2+1 || requests[2][0].Audio.StartMs != want[2].StartMs {
				t.Fatalf("committed audio cells were re-encoded: batches=%d cells=%d", len(requests), len(want))
			}
			for _, batch := range requests {
				for _, input := range batch {
					if input.Audio.URI != objects.URI(video.NormalizedVideoKey) {
						t.Fatalf("audio input lost immutable prepared artifact: %s", input.Audio.URI)
					}
				}
			}
		})
	}
}
