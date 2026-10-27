package store

import (
	"encoding/json"
	"time"

	"github.com/ozine/videohelper/internal/timeline"
)

type Video struct {
	ID                 int64                  `json:"id,string"`
	Title              string                 `json:"title"`
	Status             string                 `json:"status"`
	SourceKind         string                 `json:"source_kind"`
	SourceURL          string                 `json:"source_url,omitempty"`
	SourceFilename     string                 `json:"source_filename,omitempty"`
	SourceKey          string                 `json:"-"`
	NormalizedVideoKey string                 `json:"-"`
	Error              string                 `json:"error,omitempty"`
	DurationMs         *int                   `json:"duration_ms"`
	HasVideo           *bool                  `json:"has_video"`
	HasAudio           *bool                  `json:"has_audio"`
	Thumbs             *timeline.SpriteLayout `json:"thumbs"`
	DeletedAt          *time.Time             `json:"deleted_at,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

type Project struct {
	ID        int64     `json:"id,string"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type VideoFilter struct {
	ProjectID *int64
	IDs       []int64 // nil means all, empty means none.
	Status    string
	Deleted   bool
	Limit     int
}

type Job struct {
	ID        int64           `json:"id,string"`
	VideoID   int64           `json:"video_id,string"`
	Status    string          `json:"status"`
	Stage     string          `json:"stage"`
	Attempt   int             `json:"attempt"`
	Progress  json.RawMessage `json:"progress"`
	Error     string          `json:"error,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type MediaInfo struct {
	DurationMs         int
	HasVideo           bool
	HasAudio           bool
	NormalizedVideoKey string
	Thumbs             *timeline.SpriteLayout
}

type Word struct {
	W string `json:"w"`
	S int    `json:"s"`
	E int    `json:"e"`
}

type Sentence struct {
	VideoID int64  `json:"video_id,string"`
	Kind    string `json:"kind"`
	StartMs int    `json:"start_ms"`
	EndMs   int    `json:"end_ms"`
	Text    string `json:"text"`
	Words   []Word `json:"words,omitempty"`
	Model   string `json:"model"`
}

type Space struct {
	ID          string   `json:"id"`
	Fingerprint string   `json:"fingerprint"`
	Dim         int      `json:"dim"`
	Metric      string   `json:"metric"`
	Disabled    bool     `json:"disabled"`
	DocInputs   []string `json:"doc_inputs"`
	QueryInputs []string `json:"query_inputs"`
}

type VectorRow struct {
	VideoID     int64
	SpaceID     string
	Fingerprint string
	Channel     string
	Seg         string
	Span        timeline.Span
	Vec         []float32
}

type VectorQuery struct {
	SpaceID     string
	Fingerprint string
	Channel     string
	Seg         string
	Vec         []float32
	VideoIDs    []int64
	ProjectID   *int64
	TimeRange   *timeline.Span
	K           int
}

type VectorHit struct {
	VideoID  int64
	Span     timeline.Span
	Distance float32
	Rank     int
}
