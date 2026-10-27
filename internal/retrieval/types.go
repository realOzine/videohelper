// Package retrieval fuses ranked channel hits on a shared video timeline.
package retrieval

import "github.com/ozine/videohelper/internal/timeline"

type Config struct {
	RRFK          float64
	MergeGapCells int
}
type Hit struct {
	VideoID int64
	Span    timeline.Span
	Rank    int
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
type Sentence struct {
	VideoID int64
	Span    timeline.Span
	Text    string
}
