package search

import (
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/timeline"
)

func hit(video int64, start, end, rank int) index.VectorHit {
	return index.VectorHit{VideoID: video, Span: timeline.Span{StartMs: start, EndMs: end}, Rank: rank}
}

func TestFuseRRFBestRankAndDeterminism(t *testing.T) {
	lists := []recallList{
		{channel: "visual", space: "one", weight: 2, hits: []index.VectorHit{hit(1, 0, 1000, 2), hit(1, 0, 1000, 1)}},
		{channel: "speech", space: "one", weight: 1, hits: []index.VectorHit{hit(1, 200, 800, 2)}},
		{channel: "visual", space: "two", weight: 1, hits: []index.VectorHit{hit(1, 0, 1000, 3)}},
	}
	cfg := config.SearchConfig{RRFK: 60, MergeGapCells: 0}
	got := fuse(lists, map[int64]int{1: 1000}, nil, cfg, 20)
	want := 2.0/61 + 1.0/62 + 1.0/63
	if len(got) != 1 || math.Abs(got[0].Score-want) > 1e-12 || got[0].JumpMs != 200 || !reflect.DeepEqual(got[0].Channels, []ChannelHit{{Channel: "speech", Rank: 2}, {Channel: "visual", Rank: 1}}) {
		t.Fatalf("RRF: %+v want score %g", got, want)
	}
	slices.Reverse(lists)
	slices.Reverse(lists[2].hits)
	if shuffled := fuse(lists, map[int64]int{1: 1000}, nil, cfg, 20); !reflect.DeepEqual(shuffled, got) {
		t.Fatalf("input order changed ranking: %+v != %+v", shuffled, got)
	}
}

func TestFuseGapMaxScoreTrailingCellAndThumb(t *testing.T) {
	lists := []recallList{{channel: "visual", space: "one", weight: 1, hits: []index.VectorHit{hit(1, 0, 1000, 3), hit(1, 2000, 3000, 1), hit(1, 5000, 5500, 2)}}}
	got := fuse(lists, map[int64]int{1: 5500}, nil, config.SearchConfig{RRFK: 60, MergeGapCells: 1}, 20)
	if len(got) != 2 || got[0].Span != (timeline.Span{StartMs: 0, EndMs: 3000}) || got[0].ThumbCell != 2 || got[0].Score != 1.0/61 || got[1].Span.EndMs != 5500 {
		t.Fatalf("gap/score/thumbnail: %+v", got)
	}
}

func TestFuseHalfOpenClippingJumpAndOriginalChinese(t *testing.T) {
	lists := []recallList{
		{channel: "speech", space: "one", weight: 1, hits: []index.VectorHit{hit(1, 500, 2500, 1), hit(1, 3000, 4000, 2)}},
		{channel: "visual", space: "one", weight: 1, hits: []index.VectorHit{hit(1, 0, 1000, 1), hit(1, 2000, 3000, 2)}},
	}
	window := timeline.Span{StartMs: 1000, EndMs: 3000}
	got := fuse(lists, map[int64]int{1: 4000}, &window, config.SearchConfig{RRFK: 60, MergeGapCells: 0}, 20)
	if len(got) != 1 || got[0].Span != window || got[0].JumpMs != 1000 || got[0].ThumbCell != 2 {
		t.Fatalf("clipping/jump/thumbnail: %+v", got)
	}
	attachSnippets(got, []index.TextSpan{{VideoID: 1, Span: timeline.Span{StartMs: 500, EndMs: 2500}, Text: "这是未经改写的中文原话。"}})
	if got[0].Snippet != "这是未经改写的中文原话。" {
		t.Fatalf("original text changed: %q", got[0].Snippet)
	}
	if got[0].Channels[0].Rank != 1 {
		t.Fatalf("touching-end speech hit contributed: %+v", got[0].Channels)
	}
}

func TestFuseStableTiesAndVisualOnlySnippet(t *testing.T) {
	lists := []recallList{{channel: "visual", space: "one", weight: 1, hits: []index.VectorHit{hit(2, 0, 1000, 1), hit(1, 2000, 3000, 1), hit(1, 0, 1000, 1)}}}
	got := fuse(lists, map[int64]int{1: 3000, 2: 1000}, nil, config.SearchConfig{RRFK: 60, MergeGapCells: 0}, 2)
	if len(got) != 2 || got[0].VideoID != 1 || got[0].Span.StartMs != 0 || got[1].Span.StartMs != 2000 {
		t.Fatalf("unstable ties: %+v", got)
	}
	attachSnippets(got, []index.TextSpan{{VideoID: 1, Span: timeline.Span{StartMs: 0, EndMs: 1000}, Text: "只命中画面，也可以保留原话。"}})
	if got[0].Snippet == "" || got[1].Snippet != "" {
		t.Fatalf("snippet evidence: %+v", got)
	}
}
