package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/pipeline"
	"github.com/ozine/videohelper/internal/search"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/internal/timeline"
	"github.com/ozine/videohelper/internal/workspace"
)

type fakeSearch struct {
	call func(workspace.Principal, search.Query) (search.Result, error)
}

func (f fakeSearch) Search(_ context.Context, p workspace.Principal, q search.Query) (search.Result, error) {
	return f.call(p, q)
}

func TestSearchPreservesBusinessQueryAndNormalizesEvidence(t *testing.T) {
	for _, mode := range []string{"visual", "speech", "fused"} {
		r := request{WorkspaceID: 11, UserID: 22, VideoIDs: []int64{7, 9}, QueryID: "q1", Mode: mode, Text: " 原始中文 ", Limit: 10}
		out := searchOne(context.Background(), fakeSearch{func(p workspace.Principal, q search.Query) (search.Result, error) {
			if p.WorkspaceID != 11 || p.UserID != 22 || q.Text != r.Text || !reflect.DeepEqual(q.Scope.VideoIDs, r.VideoIDs) || q.Limit != 10 {
				t.Fatalf("changed request: %+v %+v", p, q)
			}
			if mode == "fused" && q.OnlyChannels != nil || mode != "fused" && !reflect.DeepEqual(q.OnlyChannels, []string{mode}) {
				t.Fatalf("routes %+v", q.OnlyChannels)
			}
			return search.Result{Moments: []search.Moment{{VideoID: 9, Span: timeline.Span{StartMs: 100, EndMs: 200}, JumpMs: 100, Score: .2, Channels: []search.ChannelHit{{Channel: "speech", Rank: 1}}, Snippet: "证据"}}, Skipped: []string{"audio"}}, nil
		}}, r)
		raw, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), `"video_id":9,"start_ms":100,"end_ms":200,"jump_ms":100`) || !strings.Contains(string(raw), `"channels":[{"channel":"speech","rank":1}]`) || out.Error != nil || !reflect.DeepEqual(out.Skipped, []string{"audio"}) {
			t.Fatalf("bad response %s", raw)
		}
	}
}

func TestFusedChannelWeightsForwardedWithoutChangingControls(t *testing.T) {
	weights := map[string]float64{"visual": 0, "speech": 1.5, "audio": 1}
	for _, mode := range []string{"fused", "visual", "speech"} {
		r := request{Mode: mode, ChannelWeights: weights}
		searchOne(context.Background(), fakeSearch{func(_ workspace.Principal, q search.Query) (search.Result, error) {
			if mode == "fused" {
				if !reflect.DeepEqual(q.ChannelWeights, weights) || q.OnlyChannels != nil {
					t.Fatalf("lost fused weights: %+v", q)
				}
			} else if !reflect.DeepEqual(q.ChannelWeights, map[string]float64{mode: 1}) || !reflect.DeepEqual(q.OnlyChannels, []string{mode}) {
				t.Fatalf("weights changed control: %+v", q)
			}
			return search.Result{}, nil
		}}, r)
	}
}

func TestChannelWeightValidation(t *testing.T) {
	channels, err := space.NewChannels(config.ChannelsConfig{
		"visual": {Doc: "video", Seg: "grid@v1", Weight: 1, Active: []string{"v@1"}},
		"speech": {Doc: "text", Seg: "sentence@v1", Weight: 2, Active: []string{"v@1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, weights := range []map[string]float64{nil, {"visual": 0}, {"visual": 0.5, "speech": 2}} {
		if err := (request{ChannelWeights: weights}).validateWeights(channels); err != nil {
			t.Fatalf("rejected valid weights %v: %v", weights, err)
		}
	}
	for _, weights := range []map[string]float64{{"unknown": 1}, {"visual": -1}, {"speech": math.NaN()}, {"speech": math.Inf(1)}, {"speech": math.MaxFloat64}, {"visual": 0, "speech": 0}} {
		if err := (request{ChannelWeights: weights}).validateWeights(channels); err == nil {
			t.Fatalf("accepted invalid weights %v", weights)
		}
	}
	for _, mode := range []string{"visual", "speech"} {
		r := request{WorkspaceID: 1, UserID: 2, VideoIDs: []int64{3}, QueryID: "q", Mode: mode, Text: "x", Limit: 10, ChannelWeights: map[string]float64{}}
		if err := r.validate("search"); err == nil {
			t.Fatalf("accepted weights for %s control", mode)
		}
	}
	r := request{WorkspaceID: 1, UserID: 2, VideoIDs: []int64{3}, ChannelWeights: map[string]float64{}}
	if err := r.validate("inspect"); err == nil {
		t.Fatal("inspect accepted channel_weights")
	}
}

func TestSearchErrorsKeepSkippedAndLaterQueryRuns(t *testing.T) {
	calls := 0
	f := fakeSearch{func(workspace.Principal, search.Query) (search.Result, error) {
		calls++
		if calls == 1 {
			return search.Result{Skipped: []string{"speech"}}, apperr.New(apperr.Unavailable, "search_unavailable", "failed")
		}
		return search.Result{}, nil
	}}
	r := request{QueryID: "q", Mode: "speech"}
	first := searchOne(context.Background(), f, r)
	second := searchOne(context.Background(), f, r)
	if first.Error == nil || first.Error.Code != "search_unavailable" || len(first.Skipped) != 1 || first.Moments == nil || second.Error != nil || second.Moments == nil || second.Skipped == nil {
		t.Fatalf("bad normalized results: %+v %+v", first, second)
	}
}

func TestScopeAndModeValidation(t *testing.T) {
	valid := request{WorkspaceID: 1, UserID: 2, VideoIDs: []int64{9, 3}, QueryID: "q", Mode: "fused", Text: "x", Limit: 10}
	if err := valid.validate("search"); err != nil || !reflect.DeepEqual(valid.VideoIDs, []int64{3, 9}) {
		t.Fatalf("valid scope: %v %+v", err, valid)
	}
	for _, change := range []func(*request){func(r *request) { r.VideoIDs = nil }, func(r *request) { r.VideoIDs = []int64{3, 3} }, func(r *request) { r.VideoIDs = []int64{-1} }, func(r *request) { r.WorkspaceID = 0 }, func(r *request) { r.UserID = 0 }, func(r *request) { r.Mode = "audio" }, func(r *request) { r.Text = " " }, func(r *request) { r.Limit = 0 }} {
		r := valid
		r.VideoIDs = append([]int64{}, valid.VideoIDs...)
		change(&r)
		if r.validate("search") == nil {
			t.Fatalf("accepted invalid request %+v", r)
		}
	}
	if valid.validate("inspect") == nil {
		t.Fatal("inspect accepted search fields")
	}
}

func TestCoverageUsesActualMergedASRAndExactCells(t *testing.T) {
	channels, err := space.NewChannels(config.ChannelsConfig{"visual": {Doc: "video", Seg: "grid@v1", Weight: 1, Active: []string{"v@1"}}, "speech": {Doc: "text", Seg: "sentence@v1", Weight: 1, Active: []string{"v@1"}}})
	if err != nil {
		t.Fatal(err)
	}
	duration := 2500
	yes := true
	out := inspection{Videos: []videoEvidence{{ID: 1, DurationMs: &duration, HasVideo: &yes, HasAudio: &yes}}, TextSpans: []textEvidence{{VideoID: 1, Kind: "speech", StartMs: 100, EndMs: 300, Text: "短"}, {VideoID: 1, Kind: "speech", StartMs: 400, EndMs: 900, Text: "这是后一个句子"}}}
	visual := vectorKey{1, "visual", "v@1", "grid@v1"}
	speech := vectorKey{1, "speech", "v@1", "sentence@v1"}
	actual := map[vectorKey][]timeline.Span{visual: timeline.Cells(duration), speech: {{StartMs: 100, EndMs: 900}}}
	validateCoverage(&out, channels, actual)
	if len(out.Violations) != 0 {
		t.Fatalf("valid: %+v", out.Violations)
	}
	if len(out.Coverage) != 2 || !out.Coverage[0].Matched || out.Coverage[0].Channel != "speech" || out.Coverage[0].ExpectedCount != 1 || out.Coverage[0].ActualCount != 1 || out.Coverage[1].ExpectedCount != 3 {
		t.Fatalf("bad coverage evidence: %+v", out.Coverage)
	}
	actual[speech] = []timeline.Span{{StartMs: 100, EndMs: 300}, {StartMs: 400, EndMs: 900}}
	actual[visual] = actual[visual][:2]
	out.Coverage = nil
	validateCoverage(&out, channels, actual)
	if len(out.Violations) != 2 {
		t.Fatalf("missed bad coverage: %+v", out.Violations)
	}
	if len(out.Coverage) != 2 || out.Coverage[0].Matched || out.Coverage[0].ActualCount != 2 || out.Coverage[1].ActualCount != 2 {
		t.Fatalf("bad mismatch evidence: %+v", out.Coverage)
	}
}

func TestStageGenerationAndASRIdentity(t *testing.T) {
	channels, err := space.NewChannels(config.ChannelsConfig{"speech": {Doc: "text", Seg: "sentence@v1", Weight: 1, Active: []string{"v@1"}}})
	if err != nil {
		t.Fatal(err)
	}
	duration, yes, no, key := 1000, true, false, "media"
	sp := &spaceEvidence{ID: "v@1", Dim: 2, Metric: "cosine", Fingerprint: "embed-fp"}
	spaces := map[string]*spaceEvidence{sp.ID: sp}
	models := map[string][]mlclient.TranscriberInfo{"asr@1": {{ID: "asr@1", Fingerprint: "asr-fp"}}}
	stage := func(name string, options pipeline.StageOptions) stageEvidence {
		plan, err := json.Marshal(pipeline.StageDef{Name: name, Version: "v1", Required: true, Options: options})
		if err != nil {
			t.Fatal(err)
		}
		return stageEvidence{Stage: name, Version: "v1.a2bcdefghijklmnopqrstuvwxyz", Status: "done", Required: true, Plan: plan}
	}
	base := inspection{Videos: []videoEvidence{{ID: 1, WorkspaceID: 2, Status: "ready", SourceKey: &key, NormalizedVideoKey: &key, DurationMs: &duration, HasVideo: &no, HasAudio: &yes, Stages: []stageEvidence{
		stage("probe", pipeline.StageOptions{}), stage("prepare_media", pipeline.StageOptions{}), stage("finalize", pipeline.StageOptions{}),
		stage("asr", pipeline.StageOptions{Transcriber: "asr@1", Fingerprint: "asr-fp"}), stage("embed:speech:v@1", pipeline.StageOptions{Space: space.Info{ID: sp.ID, Dim: sp.Dim, Metric: sp.Metric, Fingerprint: sp.Fingerprint}}),
	}}}, TextSpans: []textEvidence{{VideoID: 1, WorkspaceID: 2, Kind: "speech", StartMs: 100, EndMs: 900, Text: "真实转录文本", Model: "asr@1"}}}
	scope := request{WorkspaceID: 2, UserID: 3, VideoIDs: []int64{1}}
	validateVideos(&base, scope, channels, spaces, models)
	if len(base.Violations) != 0 {
		t.Fatalf("real generation rejected: %+v", base.Violations)
	}
	for _, tc := range []struct {
		name   string
		change func(*inspection)
		want   string
	}{
		{"wrong embedding generation", func(out *inspection) { out.Videos[0].Stages[4].Version = "v2.random" }, "stage generation mismatch"},
		{"missing generation", func(out *inspection) { out.Videos[0].Stages[4].Version = "v1." }, "stage generation mismatch"},
		{"ASR drift", func(out *inspection) {
			out.Videos[0].Stages[3] = stage("asr", pipeline.StageOptions{Transcriber: "asr@1", Fingerprint: "old-fp"})
		}, "ASR model identity mismatch"},
		{"ASR unknown model", func(out *inspection) {
			out.Videos[0].Stages[3] = stage("asr", pipeline.StageOptions{Transcriber: "asr@2", Fingerprint: "asr-fp"})
		}, "ASR model unavailable"},
		{"ASR transcript model", func(out *inspection) { out.TextSpans[0].Model = "other@1" }, "ASR text model mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(base)
			var out inspection
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			tc.change(&out)
			validateVideos(&out, scope, channels, spaces, models)
			if !strings.Contains(strings.Join(out.Violations, "\n"), tc.want) {
				t.Fatalf("missing %q in %+v", tc.want, out.Violations)
			}
		})
	}
}

func TestVectorAndCanonicalHash(t *testing.T) {
	for _, v := range [][]float32{{0, 0}, {1}, {1, float32(math.NaN())}, {1, float32(math.Inf(1))}} {
		if validVector(v, 2) {
			t.Fatalf("accepted %v", v)
		}
	}
	if !validVector([]float32{0, 1}, 2) {
		t.Fatal("rejected valid vector")
	}
	a, b := sha256.New(), sha256.New()
	if err := hashJSON(a, json.RawMessage(`{"a":1,"b":{"x":2,"y":3}}`)); err != nil {
		t.Fatal(err)
	}
	if err := hashJSON(b, json.RawMessage(`{ "b": {"y":3, "x":2},"a":1 }`)); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Sum(nil), b.Sum(nil)) {
		t.Fatal("JSON whitespace or key order changed fingerprint")
	}
	if err := hashJSON(b, "changed vector/text/metadata"); err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(a.Sum(nil), b.Sum(nil)) {
		t.Fatal("content mutation did not change fingerprint")
	}
}

func TestHNSWDefinitionChecksMetricDimensionAndRoute(t *testing.T) {
	sp := spaceEvidence{ID: "v@1", Dim: 2, Metric: "cosine"}
	def := `CREATE INDEX x ON public.embedding USING hnsw (((vec)::halfvec(2)) halfvec_cosine_ops)`
	predicate := `((channel = 'visual'::text) AND (space_id = 'v@1'::text))`
	if !hnswMatches(def, predicate, "visual", sp) {
		t.Fatal("valid HNSW rejected")
	}
	for _, bad := range []string{strings.ReplaceAll(def, "halfvec(2)", "halfvec(3)"), strings.ReplaceAll(def, "halfvec_cosine_ops", "halfvec_ip_ops")} {
		if hnswMatches(bad, predicate, "visual", sp) {
			t.Fatal("invalid HNSW accepted")
		}
	}
	if hnswMatches(def, predicate, "speech", sp) {
		t.Fatal("wrong channel accepted")
	}
}
