package server

import (
	"net/http"
	"strconv"

	"github.com/ozine/videohelper/internal/desktop/store"
)

func (s *Server) search(r *http.Request) (any, error) {
	var body queryJSON
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	query, err := body.query()
	if err != nil {
		return nil, err
	}
	p, err := s.engine()
	if err != nil {
		return nil, err
	}
	result, err := p.Search(r.Context(), query)
	if err != nil {
		return nil, err
	}
	videos := map[int64]store.Video{}
	rows := make([]map[string]any, 0, len(result.Moments))
	for _, m := range result.Moments {
		v, ok := videos[m.VideoID]
		if !ok {
			v, err = s.Store.Video(r.Context(), m.VideoID)
			if err != nil {
				return nil, err
			}
			videos[m.VideoID] = v
		}
		channels := make([]map[string]any, 0, len(m.Channels))
		for _, c := range m.Channels {
			channels = append(channels, map[string]any{"channel": c.Channel, "rank": c.Rank})
		}
		var snippet *string
		if m.Snippet != "" {
			snippet = &m.Snippet
		}
		rows = append(rows, map[string]any{"video_id": strconv.FormatInt(v.ID, 10), "video_title": v.Title, "start_ms": m.Span.StartMs, "end_ms": m.Span.EndMs, "jump_ms": m.JumpMs, "score": m.Score, "channels": channels, "snippet": snippet, "thumbnail": s.thumb(v, m.ThumbCell)})
	}
	return map[string]any{"moments": rows, "skipped_channels": result.Skipped}, nil
}

func (s *Server) models(r *http.Request) (any, error) {
	var body queryJSON
	if err := decode(r, &body); err != nil {
		return nil, err
	}
	query, err := body.query()
	if err != nil {
		return nil, err
	}
	p, err := s.engine()
	if err != nil {
		return nil, err
	}
	channels, err := p.Models(r.Context(), query)
	return map[string]any{"channels": channels}, err
}
