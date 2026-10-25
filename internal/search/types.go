package search

import (
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/timeline"
)

type Query struct {
	Text           string
	Scope          library.Scope
	TimeRange      *timeline.Span
	ChannelWeights map[string]float64
	ChannelQueries map[string]string
	Limit          int
	OnlyChannels   []string
	SpaceOverride  map[string]string
}

type ChannelHit struct {
	Channel string
	Rank    int
}

type Moment struct {
	VideoID   int64
	Span      timeline.Span
	JumpMs    int
	Score     float64
	Channels  []ChannelHit
	Snippet   string
	ThumbCell int
}

type Result struct {
	Moments []Moment
	Skipped []string
}
