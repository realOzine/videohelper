package search

import (
	"cmp"
	"maps"
	"slices"

	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/timeline"
)

type recallList struct {
	channel, space string
	weight         float64
	hits           []index.VectorHit
}

type rankedHit struct {
	rank  int
	span  timeline.Span
	space string
}

func betterHit(a, b rankedHit) bool {
	if b.rank == 0 {
		return true
	}
	if a.rank != b.rank {
		return a.rank < b.rank
	}
	if a.span.StartMs != b.span.StartMs {
		return a.span.StartMs < b.span.StartMs
	}
	if a.span.EndMs != b.span.EndMs {
		return a.span.EndMs < b.span.EndMs
	}
	return a.space < b.space
}

type cellKey struct {
	video int64
	cell  int
}

type scoredCell struct {
	index      int
	score      float64
	channels   map[string]int
	speech     rankedHit
	visualRank int
}

func clipped(span timeline.Span, window *timeline.Span) timeline.Span {
	if window != nil {
		span.StartMs = max(span.StartMs, window.StartMs)
		span.EndMs = min(span.EndMs, window.EndMs)
	}
	return span
}

func fuse(lists []recallList, durations map[int64]int, window *timeline.Span, cfg config.SearchConfig, limit int) []Moment {
	lists = slices.Clone(lists)
	slices.SortFunc(lists, func(a, b recallList) int {
		if order := cmp.Compare(a.channel, b.channel); order != 0 {
			return order
		}
		return cmp.Compare(a.space, b.space)
	})
	byVideo := map[int64]map[int]*scoredCell{}
	for _, list := range lists {
		best := map[cellKey]rankedHit{}
		for _, hit := range list.hits {
			if hit.Rank <= 0 {
				continue
			}
			candidate := rankedHit{rank: hit.Rank, span: hit.Span, space: list.space}
			for _, cell := range timeline.CellsOverlapping(durations[hit.VideoID], clipped(hit.Span, window)) {
				key := cellKey{video: hit.VideoID, cell: cell}
				if betterHit(candidate, best[key]) {
					best[key] = candidate
				}
			}
		}
		for key, hit := range best {
			if byVideo[key.video] == nil {
				byVideo[key.video] = map[int]*scoredCell{}
			}
			cell := byVideo[key.video][key.cell]
			if cell == nil {
				cell = &scoredCell{index: key.cell, channels: map[string]int{}}
				byVideo[key.video][key.cell] = cell
			}
			cell.score += list.weight / (cfg.RRFK + float64(hit.rank))
			if rank := cell.channels[list.channel]; rank == 0 || hit.rank < rank {
				cell.channels[list.channel] = hit.rank
			}
			if list.channel == "speech" && betterHit(hit, cell.speech) {
				cell.speech = hit
			}
			if list.channel == "visual" && (cell.visualRank == 0 || hit.rank < cell.visualRank) {
				cell.visualRank = hit.rank
			}
		}
	}
	moments := make([]Moment, 0)
	for _, video := range slices.Sorted(maps.Keys(byVideo)) {
		cells := byVideo[video]
		ordered := slices.Sorted(maps.Keys(cells))
		for start := 0; start < len(ordered); {
			end := start + 1
			for end < len(ordered) && ordered[end]-ordered[end-1]-1 <= cfg.MergeGapCells {
				end++
			}
			moments = append(moments, mergeCells(video, cells, ordered[start:end], durations[video], window))
			start = end
		}
	}
	slices.SortFunc(moments, func(a, b Moment) int {
		if order := cmp.Compare(b.Score, a.Score); order != 0 {
			return order
		}
		if order := cmp.Compare(a.VideoID, b.VideoID); order != 0 {
			return order
		}
		if order := cmp.Compare(a.Span.StartMs, b.Span.StartMs); order != 0 {
			return order
		}
		return cmp.Compare(a.Span.EndMs, b.Span.EndMs)
	})
	return moments[:min(limit, len(moments))]
}

func mergeCells(video int64, cells map[int]*scoredCell, ordered []int, duration int, window *timeline.Span) Moment {
	width := timeline.CellMs(duration)
	span := timeline.Span{StartMs: ordered[0] * width, EndMs: min((ordered[len(ordered)-1]+1)*width, duration)}
	moment := Moment{VideoID: video, Span: clipped(span, window), ThumbCell: ordered[0], Channels: []ChannelHit{}}
	channels := map[string]int{}
	var speech rankedHit
	visualRank := 0
	for _, id := range ordered {
		cell := cells[id]
		moment.Score = max(moment.Score, cell.score)
		for channel, rank := range cell.channels {
			if channels[channel] == 0 || rank < channels[channel] {
				channels[channel] = rank
			}
		}
		if cell.speech.rank > 0 && betterHit(cell.speech, speech) {
			speech = cell.speech
		}
		if cell.visualRank > 0 && (visualRank == 0 || cell.visualRank < visualRank) {
			visualRank = cell.visualRank
			moment.ThumbCell = id
		}
	}
	for _, channel := range slices.Sorted(maps.Keys(channels)) {
		moment.Channels = append(moment.Channels, ChannelHit{Channel: channel, Rank: channels[channel]})
	}
	moment.JumpMs = moment.Span.StartMs
	if speech.rank > 0 {
		moment.JumpMs = max(moment.Span.StartMs, min(speech.span.StartMs, moment.Span.EndMs-1))
	}
	return moment
}

func attachSnippets(moments []Moment, sentences []index.TextSpan) {
	byVideo := map[int64][]index.TextSpan{}
	for _, sentence := range sentences {
		byVideo[sentence.VideoID] = append(byVideo[sentence.VideoID], sentence)
	}
	for i := range moments {
		moment := &moments[i]
		for _, sentence := range byVideo[moment.VideoID] {
			if sentence.Span.StartMs <= moment.JumpMs && moment.JumpMs < sentence.Span.EndMs {
				moment.Snippet = sentence.Text
				break
			}
		}
	}
}
