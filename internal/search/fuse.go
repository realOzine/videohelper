package search

import (
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/retrieval"
	"github.com/ozine/videohelper/internal/timeline"
)

type recallList struct {
	channel, space string
	weight         float64
	hits           []index.VectorHit
}

func fuse(lists []recallList, durations map[int64]int, window *timeline.Span, cfg config.SearchConfig, limit int) []Moment {
	input := make([]retrieval.List, len(lists))
	for i, list := range lists {
		input[i] = retrieval.List{Channel: list.channel, Space: list.space, Weight: list.weight, Hits: make([]retrieval.Hit, len(list.hits))}
		for j, hit := range list.hits {
			input[i].Hits[j] = retrieval.Hit{VideoID: hit.VideoID, Span: hit.Span, Rank: hit.Rank}
		}
	}
	fused := retrieval.Fuse(input, durations, window, retrieval.Config{RRFK: cfg.RRFK, MergeGapCells: cfg.MergeGapCells}, limit)
	out := make([]Moment, len(fused))
	for i, m := range fused {
		out[i] = Moment{VideoID: m.VideoID, Span: m.Span, JumpMs: m.JumpMs, Score: m.Score, Snippet: m.Snippet, ThumbCell: m.ThumbCell, Channels: make([]ChannelHit, len(m.Channels))}
		for j, c := range m.Channels {
			out[i].Channels[j] = ChannelHit{Channel: c.Channel, Rank: c.Rank}
		}
	}
	return out
}

func attachSnippets(moments []Moment, sentences []index.TextSpan) {
	for i := range moments {
		best := -1
		for _, s := range sentences {
			if s.VideoID == moments[i].VideoID && s.Span.StartMs > best && s.Span.StartMs <= moments[i].JumpMs && moments[i].JumpMs < s.Span.EndMs {
				moments[i].Snippet = s.Text
				best = s.Span.StartMs
			}
		}
	}
}
