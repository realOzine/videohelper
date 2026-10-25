package search

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/platform/authn"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/internal/timeline"
	"github.com/ozine/videohelper/internal/workspace"
)

func TestAudioOnlySearchPreservesChineseQueryAndEvidenceIntegration(t *testing.T) {
	ctx, pool := searchDB(t)
	const id, text = "clap-htsat-unfused@r1", "  门响后有狗叫。  "
	fingerprint := "sha256:" + strings.Repeat("a", 64)
	span := 10000
	queries := make(chan mlclient.EmbedRequest, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/spaces" {
			json.NewEncoder(w).Encode(mlclient.SpacesResponse{Spaces: []mlclient.SpaceInfo{{ID: id, Dim: 512, Metric: "cosine", QueryInputs: []string{"text"}, DocInputs: []string{"audio"}, MaxSpanMs: &span, MaxBatch: 8, Fingerprint: fingerprint}}, Transcribers: []mlclient.TranscriberInfo{}})
			return
		}
		var req mlclient.EmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		queries <- req
		vector := make([]float32, 512)
		vector[0] = 1
		json.NewEncoder(w).Encode(mlclient.EmbedResponse{Space: id, Fingerprint: fingerprint, Dim: 512, Vectors: [][]float32{vector}})
	}))
	defer server.Close()
	var registry *space.Registry
	client := mlclient.New(config.MLConfig{Endpoints: []string{server.URL}, Token: "audio-contract", QueryTimeout: time.Second}, func(id string) (string, bool) { return registry.Resolve(id) })
	registry = space.New(pool, client)
	if err := registry.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := index.EnsureVectorIndex(ctx, pool, "audio", index.VectorSpace{ID: id, Dim: 512, Metric: "cosine"}); err != nil {
		t.Fatal(err)
	}
	channels, err := space.NewChannels(config.ChannelsConfig{"audio": {Doc: "audio", Seg: timeline.GridSeg, Weight: 1, Active: []string{id}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := channels.Validate(ctx, registry); err != nil {
		t.Fatal(err)
	}
	owner, err := workspace.New(pool).Resolve(ctx, authn.Claims{Sub: "audio-owner"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := workspace.New(pool).Resolve(ctx, authn.Claims{Sub: "audio-foreign"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	createVideo := func(workspaceID int64, status string, deleted bool) int64 {
		t.Helper()
		var video int64
		if err := pool.QueryRow(ctx, "INSERT INTO video(workspace_id,title,source_kind,status,duration_ms,has_audio,deleted_at) VALUES($1,'sound','upload',$2,5000,true,CASE WHEN $3 THEN now() END) RETURNING id", workspaceID, status, deleted).Scan(&video); err != nil {
			t.Fatal(err)
		}
		return video
	}
	videoID := createVideo(owner.WorkspaceID, "ready", false)
	foreignID := createVideo(foreign.WorkspaceID, "ready", false)
	rows := []index.VectorRow{}
	for _, video := range []struct{ id, workspace int64 }{
		{videoID, owner.WorkspaceID}, {foreignID, foreign.WorkspaceID},
		{createVideo(owner.WorkspaceID, "indexing", false), owner.WorkspaceID},
		{createVideo(owner.WorkspaceID, "ready", true), owner.WorkspaceID},
	} {
		vector := make([]float32, 512)
		vector[0] = 1
		rows = append(rows, index.VectorRow{VideoID: video.id, WorkspaceID: video.workspace, Channel: "audio", SpaceID: id, Seg: timeline.GridSeg, Span: timeline.Span{StartMs: 2000, EndMs: 3000}, Vec: vector})
	}
	if err := index.UpsertVectors(ctx, pool, rows); err != nil {
		t.Fatal(err)
	}
	const original = "保留的原始转录，不是声音描述。"
	if err := db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		return index.ReplaceSpeech(ctx, tx, owner.WorkspaceID, videoID, "fixture-asr", []index.TextSpan{{Span: timeline.Span{StartMs: 1500, EndMs: 2600}, Text: original}})
	}); err != nil {
		t.Fatal(err)
	}
	service := New(pool, library.New(pool, nil, library.Config{}, nil, nil), registry, channels, client, config.SearchConfig{K: 10, RRFK: 60, MaxLimit: 10})
	result, err := service.Search(ctx, owner, Query{Text: text, OnlyChannels: []string{"audio"}})
	if err != nil || len(result.Moments) != 1 || len(result.Skipped) != 0 {
		t.Fatalf("audio-only recall failed: %+v error=%v", result, err)
	}
	moment := result.Moments[0]
	if moment.VideoID != videoID || moment.JumpMs != 2000 || moment.Span != (timeline.Span{StartMs: 2000, EndMs: 3000}) || moment.ThumbCell != 2 || moment.Snippet != original || len(moment.Channels) != 1 || moment.Channels[0].Channel != "audio" || moment.Channels[0].Rank != 1 {
		t.Fatalf("audio-only evidence/scope changed: %+v", moment)
	}
	if empty, err := service.Search(ctx, owner, Query{Text: text, Scope: library.Scope{VideoIDs: []int64{}}}); err != nil || len(empty.Moments) != 0 || len(queries) != 1 {
		t.Fatalf("empty audio scope encoded or returned results: %+v error=%v", empty, err)
	}
	if _, err := service.Search(ctx, owner, Query{Text: text, Scope: library.Scope{VideoIDs: []int64{foreignID}}}); apperr.KindOf(err) != apperr.NotFound {
		t.Fatalf("foreign audio scope=%v", err)
	}
	if empty, err := service.Search(ctx, owner, Query{Text: text, TimeRange: &timeline.Span{StartMs: 3000, EndMs: 4000}}); err != nil || len(empty.Moments) != 0 {
		t.Fatalf("touching audio interval contributed: %+v error=%v", empty, err)
	}
	if len(queries) != 2 {
		t.Fatalf("unexpected audio query encoding count=%d", len(queries))
	}
	for range 2 {
		request := <-queries
		if request.Space != id || request.Role != "query" || len(request.Inputs) != 1 || request.Inputs[0].Text != text || request.Inputs[0].Audio != nil || request.Inputs[0].Video != nil {
			t.Fatalf("original Chinese query was altered or became media input: %+v", request)
		}
	}
}
