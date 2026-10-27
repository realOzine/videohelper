package retrieval

import (
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/ozine/videohelper/internal/timeline"
)

func hit(video int64, start, end, rank int) Hit {
	return Hit{VideoID: video, Span: timeline.Span{StartMs: start, EndMs: end}, Rank: rank}
}

func TestFuseRRFBestRankAndDeterminism(t *testing.T) {
	lists := []List{
		{Channel: "visual", Space: "one", Weight: 2, Hits: []Hit{hit(1, 0, 1000, 2), hit(1, 0, 1000, 1)}},
		{Channel: "speech", Space: "one", Weight: 1, Hits: []Hit{hit(1, 200, 800, 2)}},
		{Channel: "visual", Space: "two", Weight: 1, Hits: []Hit{hit(1, 0, 1000, 3)}},
	}
	cfg := Config{RRFK: 60, MergeGapCells: 0}
	got := Fuse(lists, map[int64]int{1: 1000}, nil, cfg, 20)
	want := 2.0/61 + 1.0/62 + 1.0/63
	if len(got) != 1 || math.Abs(got[0].Score-want) > 1e-12 || got[0].JumpMs != 200 || !reflect.DeepEqual(got[0].Channels, []ChannelHit{{Channel: "speech", Rank: 2}, {Channel: "visual", Rank: 1}}) {
		t.Fatalf("RRF: %+v want score %g", got, want)
	}
	slices.Reverse(lists)
	slices.Reverse(lists[2].Hits)
	if shuffled := Fuse(lists, map[int64]int{1: 1000}, nil, cfg, 20); !reflect.DeepEqual(shuffled, got) {
		t.Fatalf("input order changed ranking: %+v != %+v", shuffled, got)
	}
}

func TestFuseGapMaxScoreTrailingCellAndThumb(t *testing.T) {
	lists := []List{{Channel: "visual", Space: "one", Weight: 1, Hits: []Hit{hit(1, 0, 1000, 3), hit(1, 2000, 3000, 1), hit(1, 5000, 5500, 2)}}}
	got := Fuse(lists, map[int64]int{1: 5500}, nil, Config{RRFK: 60, MergeGapCells: 1}, 20)
	if len(got) != 2 || got[0].Span != (timeline.Span{StartMs: 0, EndMs: 3000}) || got[0].ThumbCell != 2 || got[0].Score != 1.0/61 || got[1].Span.EndMs != 5500 {
		t.Fatalf("gap/score/thumbnail: %+v", got)
	}
}

func TestFuseHalfOpenClippingJumpAndOriginalChinese(t *testing.T) {
	lists := []List{
		{Channel: "speech", Space: "one", Weight: 1, Hits: []Hit{hit(1, 500, 2500, 1), hit(1, 3000, 4000, 2)}},
		{Channel: "visual", Space: "one", Weight: 1, Hits: []Hit{hit(1, 0, 1000, 1), hit(1, 2000, 3000, 2)}},
	}
	window := timeline.Span{StartMs: 1000, EndMs: 3000}
	got := Fuse(lists, map[int64]int{1: 4000}, &window, Config{RRFK: 60, MergeGapCells: 0}, 20)
	if len(got) != 1 || got[0].Span != window || got[0].JumpMs != 1000 || got[0].ThumbCell != 2 {
		t.Fatalf("clipping/jump/thumbnail: %+v", got)
	}
	AttachSnippets(got, []Sentence{{VideoID: 1, Span: timeline.Span{StartMs: 500, EndMs: 2500}, Text: "这是未经改写的中文原话。"}})
	if got[0].Snippet != "这是未经改写的中文原话。" {
		t.Fatalf("original text changed: %q", got[0].Snippet)
	}
	if got[0].Channels[0].Rank != 1 {
		t.Fatalf("touching-end speech hit contributed: %+v", got[0].Channels)
	}
}

func TestFuseStableTiesAndVisualOnlySnippet(t *testing.T) {
	lists := []List{{Channel: "visual", Space: "one", Weight: 1, Hits: []Hit{hit(2, 0, 1000, 1), hit(1, 2000, 3000, 1), hit(1, 0, 1000, 1)}}}
	got := Fuse(lists, map[int64]int{1: 3000, 2: 1000}, nil, Config{RRFK: 60, MergeGapCells: 0}, 2)
	if len(got) != 2 || got[0].VideoID != 1 || got[0].Span.StartMs != 0 || got[1].Span.StartMs != 2000 {
		t.Fatalf("unstable ties: %+v", got)
	}
	AttachSnippets(got, []Sentence{{VideoID: 1, Span: timeline.Span{StartMs: 0, EndMs: 1000}, Text: "只命中画面，也可以保留原话。"}})
	if got[0].Snippet == "" || got[1].Snippet != "" {
		t.Fatalf("snippet evidence: %+v", got)
	}
}

func TestAttachSnippetsOverlappingSentences(t *testing.T) {
	sentences := []Sentence{
		{VideoID: 1, Span: timeline.Span{StartMs: 1000, EndMs: 2000}, Text: "嵌套句"},
		{VideoID: 1, Span: timeline.Span{StartMs: 0, EndMs: 3000}, Text: "前句"},
		{VideoID: 1, Span: timeline.Span{StartMs: 1800, EndMs: 2500}, Text: "重叠句"},
		{VideoID: 1, Span: timeline.Span{StartMs: 1800, EndMs: 2400}, Text: "同起点保留首句"},
		{VideoID: 2, Span: timeline.Span{StartMs: 1900, EndMs: 4000}, Text: "其他视频"},
	}
	moments := []Moment{{VideoID: 1, JumpMs: 0}, {VideoID: 1, JumpMs: 1000}, {VideoID: 1, JumpMs: 1800}, {VideoID: 1, JumpMs: 2000}, {VideoID: 1, JumpMs: 2500}, {VideoID: 1, JumpMs: 3000}}
	AttachSnippets(moments, sentences)
	for i, want := range []string{"前句", "嵌套句", "重叠句", "重叠句", "前句", ""} {
		if moments[i].Snippet != want {
			t.Errorf("jump=%d snippet=%q want=%q", moments[i].JumpMs, moments[i].Snippet, want)
		}
	}
}
