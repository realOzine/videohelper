package httpapi

import (
	"net/http"
	"strconv"

	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/search"
	"github.com/ozine/videohelper/internal/timeline"
)

type rangeJSON struct {
	StartMs int `json:"start_ms"`
	EndMs   int `json:"end_ms"`
}
type thumbnail struct {
	URL string `json:"url"`
	X   int    `json:"x"`
	Y   int    `json:"y"`
	W   int    `json:"w"`
	H   int    `json:"h"`
}
type channelHit struct {
	Channel string `json:"channel"`
	Rank    int    `json:"rank"`
}
type momentResponse struct {
	VideoID    string       `json:"video_id"`
	VideoTitle string       `json:"video_title"`
	StartMs    int          `json:"start_ms"`
	EndMs      int          `json:"end_ms"`
	JumpMs     int          `json:"jump_ms"`
	Score      float64      `json:"score"`
	Channels   []channelHit `json:"channels"`
	Snippet    *string      `json:"snippet"`
	Thumbnail  *thumbnail   `json:"thumbnail"`
}

func (a api) search(r *http.Request) (int, any, error) {
	var body struct {
		Query     string     `json:"query"`
		ProjectID *string    `json:"project_id"`
		VideoIDs  []string   `json:"video_ids"`
		TimeRange *rangeJSON `json:"time_range"`
		Limit     int        `json:"limit"`
	}
	if err := decode(r, &body, false); err != nil {
		return 0, nil, err
	}
	project, err := optionalID(body.ProjectID)
	if err != nil {
		return 0, nil, err
	}
	query := search.Query{Text: body.Query, Scope: library.Scope{ProjectID: project}, Limit: body.Limit}
	if body.VideoIDs != nil {
		query.Scope.VideoIDs = make([]int64, len(body.VideoIDs))
		for i, raw := range body.VideoIDs {
			id, err := parseID(raw)
			if err != nil {
				return 0, nil, err
			}
			query.Scope.VideoIDs[i] = id
		}
	}
	if body.TimeRange != nil {
		query.TimeRange = &timeline.Span{StartMs: body.TimeRange.StartMs, EndMs: body.TimeRange.EndMs}
	}
	result, err := a.Search.Search(r.Context(), principal(r), query)
	if err != nil {
		return 0, nil, err
	}
	videos := map[int64]library.Video{}
	urls := map[string]string{}
	moments := make([]momentResponse, 0, len(result.Moments))
	for _, moment := range result.Moments {
		video, ok := videos[moment.VideoID]
		if !ok {
			video, err = a.Library.Get(r.Context(), principal(r), moment.VideoID)
			if err != nil {
				return 0, nil, err
			}
			videos[moment.VideoID] = video
		}
		public := momentResponse{VideoID: strconv.FormatInt(moment.VideoID, 10), VideoTitle: video.Title, StartMs: moment.Span.StartMs, EndMs: moment.Span.EndMs, JumpMs: moment.JumpMs, Score: moment.Score, Channels: make([]channelHit, 0, len(moment.Channels))}
		for _, hit := range moment.Channels {
			public.Channels = append(public.Channels, channelHit{Channel: hit.Channel, Rank: hit.Rank})
		}
		if moment.Snippet != "" {
			snippet := moment.Snippet
			public.Snippet = &snippet
		}
		if video.Thumbs != nil && video.NormalizedVideoKey != "" && a.Blob != nil {
			sprite, x, y := video.Thumbs.Tile(moment.ThumbCell)
			if sprite >= 0 {
				key := blob.SpriteKeyForVideo(video.NormalizedVideoKey, sprite)
				url, ok := urls[key]
				if !ok {
					url, err = a.Blob.PresignGet(r.Context(), key, a.Config.PresignTTL)
					if err != nil {
						return 0, nil, err
					}
					urls[key] = url
				}
				public.Thumbnail = &thumbnail{URL: url, X: x, Y: y, W: video.Thumbs.TileW, H: video.Thumbs.TileH}
			}
		}
		moments = append(moments, public)
	}
	return 200, map[string]any{"moments": moments, "skipped_channels": result.Skipped}, nil
}
