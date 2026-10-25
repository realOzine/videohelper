package mlclient

type SpaceInfo struct {
	ID          string   `json:"id"`
	Dim         int      `json:"dim"`
	Metric      string   `json:"metric"`
	QueryInputs []string `json:"query_inputs"`
	DocInputs   []string `json:"doc_inputs"`
	MaxSpanMs   *int     `json:"max_span_ms"`
	MaxBatch    int      `json:"max_batch"`
	Fingerprint string   `json:"fingerprint"`
}

type TranscriberInfo struct {
	ID          string   `json:"id"`
	Languages   []string `json:"languages"`
	MaxWindowMs int      `json:"max_window_ms"`
	Fingerprint string   `json:"fingerprint"`
}

type SpacesResponse struct {
	Spaces       []SpaceInfo       `json:"spaces"`
	Transcribers []TranscriberInfo `json:"transcribers"`
}

type Discovery struct {
	Endpoints map[string]SpacesResponse
	Errors    map[string]error
}

type MediaRef struct {
	URI     string `json:"uri"`
	StartMs int    `json:"start_ms"`
	EndMs   int    `json:"end_ms"`
}

type ImageRef struct {
	URI string `json:"uri"`
}

type EmbedInput struct {
	Text  string    `json:"text,omitempty"`
	Video *MediaRef `json:"video,omitempty"`
	Audio *MediaRef `json:"audio,omitempty"`
	Image *ImageRef `json:"image,omitempty"`
}

type EmbedRequest struct {
	Space  string       `json:"space"`
	Role   string       `json:"role"`
	Inputs []EmbedInput `json:"inputs"`
}

type EmbedResponse struct {
	Space       string      `json:"space"`
	Fingerprint string      `json:"fingerprint"`
	Dim         int         `json:"dim"`
	Vectors     [][]float32 `json:"vectors"`
}

type TranscribeRequest struct {
	Model    string `json:"model"`
	URI      string `json:"uri"`
	Language string `json:"language"`
	StartMs  int    `json:"start_ms"`
	EndMs    int    `json:"end_ms"`
}

type Word struct {
	W string `json:"w"`
	S int    `json:"s"`
	E int    `json:"e"`
}

type Sentence struct {
	StartMs int    `json:"start_ms"`
	EndMs   int    `json:"end_ms"`
	Text    string `json:"text"`
	Words   []Word `json:"words"`
}

type TranscribeResponse struct {
	Model       string     `json:"model"`
	Fingerprint string     `json:"fingerprint"`
	Language    string     `json:"language"`
	DurationMs  int        `json:"duration_ms"`
	Sentences   []Sentence `json:"sentences"`
}
