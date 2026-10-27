// Package processor runs the desktop's durable media jobs and local search.
package processor

import (
	"time"

	desktopruntime "github.com/ozine/videohelper/internal/desktop/runtime"
	"github.com/ozine/videohelper/internal/retrieval"
	"github.com/ozine/videohelper/internal/timeline"
)

type Config struct {
	Tools       desktopruntime.Tools
	Endpoints   []string
	Token       string
	VisualSpace string
	AudioSpace  string
	Transcriber string
}

type Query struct {
	Text           string
	ProjectID      *int64
	VideoIDs       []int64
	TimeRange      *timeline.Span
	ChannelWeights map[string]float64
	ChannelQueries map[string]string
	Limit          int
	OnlyChannels   []string
	SpaceOverride  map[string]string
}

type ChannelHit = retrieval.ChannelHit
type Moment = retrieval.Moment
type Result struct {
	Moments []Moment
	Skipped []string
}

type ChannelModels struct {
	Channel         string        `json:"channel"`
	DocInput        string        `json:"doc_input"`
	DefaultSpaceIDs []string      `json:"default_space_ids"`
	DefaultWeight   float64       `json:"default_weight"`
	Models          []ModelOption `json:"models"`
}
type ModelOption struct {
	SpaceID        string   `json:"space_id"`
	DocInputs      []string `json:"doc_inputs"`
	QueryInputs    []string `json:"query_inputs"`
	Status         string   `json:"status"`
	EligibleVideos int      `json:"eligible_videos"`
	IndexedVideos  int      `json:"indexed_videos"`
}
type StageStatus struct {
	Stage     string    `json:"stage"`
	Version   string    `json:"version"`
	Status    string    `json:"status"`
	Attempt   int       `json:"attempt"`
	Required  bool      `json:"required"`
	Progress  *float64  `json:"progress,omitempty"`
	Error     string    `json:"error,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}
