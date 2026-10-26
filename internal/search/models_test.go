package search

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/platform/authn"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/internal/timeline"
	"github.com/ozine/videohelper/internal/workspace"
)

func TestModelCandidatesAndUnavailableStatus(t *testing.T) {
	channel := space.Channel{Name: "visual", Doc: "video", Active: []string{"missing@r1"}, Shadow: []string{"wrong@r1"}}
	infos := []space.Info{
		{ID: "multi@r1", Fingerprint: "fp", Endpoint: "http://ml", DocInputs: []string{"video", "audio", "text"}, QueryInputs: []string{"text"}, MaxSpanMs: 2500},
		{ID: "wrong@r1", Fingerprint: "fp", DocInputs: []string{"text"}, QueryInputs: []string{"text"}},
		{ID: "short@r1", Fingerprint: "fp", DocInputs: []string{"video"}, QueryInputs: []string{"text"}, MaxSpanMs: 2499},
		{ID: "image@r1", Fingerprint: "fp", DocInputs: []string{"video"}, QueryInputs: []string{"image"}, MaxSpanMs: 2500},
	}
	candidates := modelCandidates(channel, infos)
	ids := make([]string, 0)
	for _, info := range candidates {
		ids = append(ids, info.ID)
	}
	if !slices.Equal(ids, []string{"missing@r1", "multi@r1", "wrong@r1"}) {
		t.Fatalf("candidate filtering=%v", ids)
	}
	service := &Service{}
	for _, test := range []struct {
		info space.Info
		want string
	}{
		{info: candidates[0], want: "unavailable"},
		{info: candidates[2], want: "incompatible"},
		{info: func() space.Info { x := infos[0]; x.Disabled = true; return x }(), want: "disabled"},
		{info: func() space.Info { x := infos[0]; x.Endpoint = ""; return x }(), want: "unavailable"},
	} {
		option, err := service.modelOption(context.Background(), workspace.Principal{}, nil, channel, test.info)
		if err != nil || option.Status != test.want || option.DocInputs == nil || option.QueryInputs == nil {
			t.Fatalf("early status=%+v err=%v want=%s", option, err, test.want)
		}
	}
}

func TestModelsAuthenticationAndEmptyScopeOverrides(t *testing.T) {
	registry := space.New(nil, nil, config.MLConfig{})
	service := New(nil, library.New(nil, nil, library.Config{}, nil, nil), registry, testChannels(t), nil, config.SearchConfig{MaxLimit: 50})
	if _, err := service.Models(context.Background(), workspace.Principal{}, library.Scope{}); apperr.KindOf(err) != apperr.Unauthenticated {
		t.Fatalf("anonymous catalog=%v", err)
	}
	p := workspace.Principal{UserID: 1, WorkspaceID: 1}
	result, err := service.Models(context.Background(), p, library.Scope{VideoIDs: []int64{}})
	if err != nil || len(result) != 2 || result[0].Models[0].Status != "unavailable" {
		t.Fatalf("missing configured models catalog=%+v err=%v", result, err)
	}
	for _, test := range []struct {
		id   string
		want apperr.Kind
	}{{id: "unknown@r1", want: apperr.Invalid}, {id: "contract-test@r1", want: apperr.Unavailable}} {
		q := Query{Text: "测试", Scope: library.Scope{VideoIDs: []int64{}}, ChannelWeights: map[string]float64{"visual": 0}, SpaceOverride: map[string]string{"visual": test.id}}
		if _, err := service.Search(context.Background(), p, q); apperr.KindOf(err) != test.want {
			t.Fatalf("empty scope zero-weight override %s=%v", test.id, err)
		}
	}
}

func TestModelsAndExplicitOverridesIntegration(t *testing.T) {
	ctx, pool := searchDB(t)
	fingerprint := "sha256:" + strings.Repeat("a", 64)
	span, short := 30000, 2499
	metadata := mlclient.SpacesResponse{Spaces: []mlclient.SpaceInfo{}, Transcribers: []mlclient.TranscriberInfo{{ID: "asr@r1", Fingerprint: fingerprint, Languages: []string{"zh"}, MaxWindowMs: 10000}}}
	for _, id := range []string{"default@r1", "alternate@r1", "disabled@r1", "offline@r1", "wrong@r1", "short@r1", "image-query@r1"} {
		info := mlclient.SpaceInfo{ID: id, Dim: 3, Metric: "cosine", Fingerprint: fingerprint, DocInputs: []string{"text", "video", "audio"}, QueryInputs: []string{"text"}, MaxSpanMs: &span, MaxBatch: 8}
		switch id {
		case "wrong@r1":
			info.DocInputs = []string{"text"}
		case "short@r1":
			info.MaxSpanMs = &short
		case "image-query@r1":
			info.QueryInputs = []string{"image"}
		}
		metadata.Spaces = append(metadata.Spaces, info)
	}
	requests := make(chan string, 20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/spaces" {
			json.NewEncoder(w).Encode(metadata)
			return
		}
		var req mlclient.EmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		requests <- req.Space
		json.NewEncoder(w).Encode(mlclient.EmbedResponse{Space: req.Space, Fingerprint: fingerprint, Dim: 3, Vectors: [][]float32{{1, 0, 0}}})
	}))
	defer server.Close()
	cfg := config.MLConfig{Token: "catalog-contract", Endpoints: []string{server.URL}, ModelEndpoints: map[string]string{"offline@r1": "http://unavailable"}}
	var registry *space.Registry
	client := mlclient.New(cfg, func(id string) (string, bool) { return registry.Resolve(id) })
	registry = space.New(pool, client, cfg)
	if err := registry.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE embedding_space SET disabled=true WHERE id='disabled@r1'"); err != nil {
		t.Fatal(err)
	}
	if err := registry.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	channels, err := space.NewChannels(config.ChannelsConfig{
		"visual": {Doc: "video", Seg: timeline.GridSeg, Weight: 2, Active: []string{"default@r1"}, Shadow: []string{"missing@r1", "wrong@r1", "short@r1"}},
		"speech": {Doc: "text", Seg: timeline.SentenceSeg, Weight: 1, Active: []string{"default@r1"}},
		"audio":  {Doc: "audio", Seg: timeline.GridSeg, Weight: 1, Active: []string{"default@r1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := workspace.New(pool).Resolve(ctx, authn.Claims{Sub: "catalog-owner"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := workspace.New(pool).Resolve(ctx, authn.Claims{Sub: "catalog-foreign"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	create := func(workspaceID int64, audio bool) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, "INSERT INTO video(workspace_id,title,source_kind,status,duration_ms,has_video,has_audio) VALUES($1,'catalog','upload','ready',5000,true,$2) RETURNING id", workspaceID, audio).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	v1, v2, foreignID := create(p.WorkspaceID, true), create(p.WorkspaceID, false), create(foreign.WorkspaceID, true)
	lb := library.New(pool, nil, library.Config{}, nil, nil)
	service := New(pool, lb, registry, channels, client, config.SearchConfig{K: 10, RRFK: 60, MaxLimit: 50, ChannelWeights: map[string]float64{"audio": 0, "visual": 0.5}})
	catalog := func(scope library.Scope) map[string]map[string]ModelOption {
		t.Helper()
		result, err := service.Models(ctx, p, scope)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(result)
		if strings.Contains(string(encoded), server.URL) || strings.Contains(string(encoded), "endpoint") || strings.Contains(string(encoded), "fingerprint") {
			t.Fatalf("private registry metadata leaked: %s", encoded)
		}
		out := map[string]map[string]ModelOption{}
		for _, channel := range result {
			out[channel.Channel] = map[string]ModelOption{}
			wantWeight := map[string]float64{"audio": 0, "speech": 1, "visual": 0.5}[channel.Channel]
			if channel.DefaultWeight != wantWeight || !slices.Equal(channel.DefaultSpaceIDs, []string{"default@r1"}) {
				t.Fatalf("catalog defaults=%+v", channel)
			}
			for _, option := range channel.Models {
				out[channel.Channel][option.SpaceID] = option
			}
		}
		return out
	}
	options := catalog(library.Scope{})
	for id, want := range map[string]string{"default@r1": "index_missing", "alternate@r1": "index_missing", "disabled@r1": "disabled", "offline@r1": "unavailable", "missing@r1": "unavailable", "wrong@r1": "incompatible", "short@r1": "incompatible"} {
		if got := options["visual"][id].Status; got != want {
			t.Fatalf("visual %s status=%s want=%s", id, got, want)
		}
	}
	for _, channel := range []string{"audio", "speech", "visual"} {
		if _, ok := options[channel]["alternate@r1"]; !ok {
			t.Fatalf("multimodal space absent from %s", channel)
		}
		if _, ok := options[channel]["asr@r1"]; ok {
			t.Fatalf("ASR entered %s candidates", channel)
		}
		if _, ok := options[channel]["image-query@r1"]; ok {
			t.Fatalf("non-text query model entered %s candidates", channel)
		}
	}
	for _, test := range []struct {
		id   string
		kind apperr.Kind
	}{{"unknown@r1", apperr.Invalid}, {"wrong@r1", apperr.Invalid}, {"short@r1", apperr.Invalid}, {"offline@r1", apperr.Unavailable}, {"disabled@r1", apperr.Unavailable}, {"alternate@r1", apperr.Conflict}} {
		_, err := service.Search(ctx, p, Query{Text: "原文", OnlyChannels: []string{"audio"}, ChannelWeights: map[string]float64{"visual": 0}, SpaceOverride: map[string]string{"visual": test.id}})
		if apperr.KindOf(err) != test.kind {
			t.Fatalf("override %s error=%v want=%v", test.id, err, test.kind)
		}
		if test.kind == apperr.Conflict && err.(*apperr.Error).Code != "not_ready" {
			t.Fatalf("readiness code=%v", err)
		}
	}
	for _, channel := range channels.List() {
		if err := index.EnsureVectorIndex(ctx, pool, channel.Name, index.VectorSpace{ID: "alternate@r1", Dim: 3, Metric: "cosine"}); err != nil {
			t.Fatal(err)
		}
	}
	markDone := func(videoID int64, channel, seg string) {
		t.Helper()
		output, err := json.Marshal(map[string]any{"plan": map[string]any{"options": map[string]any{"channel": channel, "seg": seg, "space": space.Info{ID: "alternate@r1", Fingerprint: fingerprint}}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "INSERT INTO stage_run(video_id,stage,version,status,output) VALUES($1,$2,'v1','done',$3)", videoID, "embed:"+channel+":alternate@r1", output); err != nil {
			t.Fatal(err)
		}
	}
	markDone(v1, "visual", timeline.GridSeg)
	options = catalog(library.Scope{})
	if option := options["visual"]["alternate@r1"]; option.Status != "indexing" || option.EligibleVideos != 2 || option.IndexedVideos != 1 {
		t.Fatalf("partial coverage=%+v", option)
	}
	if _, err := service.Search(ctx, p, Query{Text: "原文", TimeRange: &timeline.Span{StartMs: 0, EndMs: 1}, SpaceOverride: map[string]string{"visual": "alternate@r1"}}); apperr.KindOf(err) != apperr.Conflict {
		t.Fatalf("time range bypassed incomplete coverage: %v", err)
	}
	if option := catalog(library.Scope{VideoIDs: []int64{v1}})["visual"]["alternate@r1"]; option.Status != "ready" || option.EligibleVideos != 1 || option.IndexedVideos != 1 {
		t.Fatalf("selected-video coverage=%+v", option)
	}
	empty, err := lb.CreateProject(ctx, p, "empty")
	if err != nil {
		t.Fatal(err)
	}
	if option := catalog(library.Scope{ProjectID: &empty.ID})["visual"]["alternate@r1"]; option.Status != "ready" || option.EligibleVideos != 0 || option.IndexedVideos != 0 {
		t.Fatalf("empty-project coverage=%+v", option)
	}
	if option := catalog(library.Scope{VideoIDs: []int64{}})["visual"]["default@r1"]; option.Status != "index_missing" {
		t.Fatalf("empty scope concealed missing physical index=%+v", option)
	}
	if _, err := service.Models(ctx, p, library.Scope{VideoIDs: []int64{foreignID}}); apperr.KindOf(err) != apperr.NotFound {
		t.Fatalf("foreign catalog scope=%v", err)
	}
	foreignProject, err := lb.CreateProject(ctx, foreign, "foreign")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Models(ctx, p, library.Scope{ProjectID: &foreignProject.ID}); apperr.KindOf(err) != apperr.NotFound {
		t.Fatalf("foreign project catalog scope=%v", err)
	}
	markDone(v2, "visual", timeline.GridSeg)
	markDone(v1, "speech", timeline.SentenceSeg)
	markDone(v1, "audio", timeline.GridSeg)
	options = catalog(library.Scope{})
	for _, channel := range []string{"audio", "speech", "visual"} {
		option := options[channel]["alternate@r1"]
		if option.Status != "ready" || option.EligibleVideos != option.IndexedVideos || option.IndexedVideos == 0 {
			t.Fatalf("complete %s coverage=%+v", channel, option)
		}
	}
	if err := index.UpsertVectors(ctx, pool, []index.VectorRow{{VideoID: v2, WorkspaceID: p.WorkspaceID, Channel: "visual", Seg: timeline.GridSeg, SpaceID: "alternate@r1", Span: timeline.Span{StartMs: 3000, EndMs: 4000}, Vec: []float32{1, 0, 0}}}); err != nil {
		t.Fatal(err)
	}
	result, err := service.Search(ctx, p, Query{Text: "原文", OnlyChannels: []string{"visual"}, SpaceOverride: map[string]string{"visual": "alternate@r1"}})
	if err != nil || len(result.Moments) != 1 || result.Moments[0].VideoID != v2 || len(requests) != 1 || <-requests != "alternate@r1" {
		t.Fatalf("alternate did not route matching vectors: %+v err=%v", result, err)
	}
	if result, err := service.Search(ctx, p, Query{Text: "原文", OnlyChannels: []string{"audio"}, SpaceOverride: map[string]string{"audio": "alternate@r1"}}); err != nil || len(result.Moments) != 0 || len(requests) != 0 {
		t.Fatalf("model selection enabled default-zero audio: %+v err=%v", result, err)
	}
	// Defaults retain their existing search path even while catalog readiness is incomplete.
	if _, err := service.Search(ctx, p, Query{Text: "原文"}); err != nil {
		t.Fatalf("default search gained readiness enforcement: %v", err)
	}
	if len(requests) != 1 || <-requests != "default@r1" {
		t.Fatal("default model routing changed or disabled audio was enabled")
	}
	if !slices.Equal(slices.Sorted(maps.Keys(options["visual"])), []string{"alternate@r1", "default@r1", "disabled@r1", "missing@r1", "offline@r1", "short@r1", "wrong@r1"}) {
		t.Fatalf("unexpected visual candidates=%v", maps.Keys(options["visual"]))
	}
}
