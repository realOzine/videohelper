package processor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/ozine/videohelper/internal/desktop/store"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/retrieval"
	"github.com/ozine/videohelper/internal/timeline"
)

func validChannel(channel string) bool {
	return channel == "visual" || channel == "speech" || channel == "audio"
}
func validateScope(q Query) error {
	if q.ProjectID != nil && *q.ProjectID <= 0 {
		return store.ErrInvalid
	}
	for _, id := range q.VideoIDs {
		if id <= 0 {
			return store.ErrInvalid
		}
	}
	return nil
}

func validateQuery(q *Query) error {
	if err := validateScope(*q); err != nil {
		return err
	}
	if q.Limit == 0 {
		q.Limit = 20
	}
	if strings.TrimSpace(q.Text) == "" || q.Limit < 1 || q.Limit > 100 {
		return store.ErrInvalid
	}
	if q.TimeRange != nil && (q.TimeRange.StartMs < 0 || q.TimeRange.EndMs <= q.TimeRange.StartMs || q.TimeRange.EndMs > math.MaxInt32) {
		return store.ErrInvalid
	}
	for _, c := range q.OnlyChannels {
		if !validChannel(c) {
			return store.ErrInvalid
		}
	}
	for c, w := range q.ChannelWeights {
		if !validChannel(c) || math.IsNaN(w) || math.IsInf(w, 0) || w < 0 {
			return store.ErrInvalid
		}
	}
	for c, text := range q.ChannelQueries {
		if !validChannel(c) || strings.TrimSpace(text) == "" {
			return store.ErrInvalid
		}
	}
	for c, id := range q.SpaceOverride {
		if !validChannel(c) || id == "" {
			return store.ErrInvalid
		}
	}
	return nil
}

func (p *Processor) scopedVideos(ctx context.Context, q Query) ([]store.Video, error) {
	if err := validateScope(q); err != nil {
		return nil, err
	}
	return p.store.ListVideos(ctx, store.VideoFilter{ProjectID: q.ProjectID, IDs: q.VideoIDs, Status: "ready"})
}

func (p *Processor) coverage(ctx context.Context, videos []store.Video, r route) (int, int, error) {
	eligibleCount, indexed := 0, 0
	for _, v := range videos {
		if !eligible(v, r.channel) {
			continue
		}
		eligibleCount++
		j, err := p.store.LatestJob(ctx, v.ID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return 0, 0, err
		}
		var cp checkpoint
		if err := json.Unmarshal(j.Progress, &cp); err != nil {
			return 0, 0, err
		}
		if j.Status == "done" && cp.Identity[r.channel] == r.localID && cp.Done["embed:"+r.channel] {
			indexed++
		}
	}
	return eligibleCount, indexed, nil
}

func (p *Processor) Models(ctx context.Context, q Query) ([]ChannelModels, error) {
	videos, err := p.scopedVideos(ctx, q)
	if err != nil {
		return nil, err
	}
	channels := make([]ChannelModels, 0, len(p.routes))
	for _, r := range p.routes {
		eligible, indexed, err := p.coverage(ctx, videos, r)
		if err != nil {
			return nil, err
		}
		status := "ready"
		if indexed < eligible {
			status = "indexing"
		}
		channels = append(channels, ChannelModels{Channel: r.channel, DocInput: r.doc, DefaultSpaceIDs: []string{r.localID}, DefaultWeight: r.weight, Models: []ModelOption{{SpaceID: r.localID, DocInputs: slices.Clone(r.info.DocInputs), QueryInputs: slices.Clone(r.info.QueryInputs), Status: status, EligibleVideos: eligible, IndexedVideos: indexed}}})
	}
	return channels, nil
}

func (p *Processor) Search(ctx context.Context, q Query) (Result, error) {
	result := Result{Moments: []Moment{}, Skipped: []string{}}
	if err := validateQuery(&q); err != nil {
		return result, err
	}
	videos, err := p.scopedVideos(ctx, q)
	if err != nil {
		return result, err
	}
	if len(videos) == 0 {
		return result, nil
	}
	durations := map[int64]int{}
	for _, v := range videos {
		if v.DurationMs != nil {
			durations[v.ID] = *v.DurationMs
		}
	}
	lists := []retrieval.List{}
	encoded := map[string][]float32{}
	failures := []error{}
	active := 0
	for _, r := range p.routes {
		if len(q.OnlyChannels) > 0 && !slices.Contains(q.OnlyChannels, r.channel) {
			continue
		}
		weight := r.weight
		if override, ok := q.ChannelWeights[r.channel]; ok {
			weight = override
		}
		if weight == 0 {
			continue
		}
		active++
		if id, ok := q.SpaceOverride[r.channel]; ok {
			if id != r.localID {
				return result, fmt.Errorf("所选模型当前未运行: %w", store.ErrConflict)
			}
			eligible, indexed, err := p.coverage(ctx, videos, r)
			if err != nil {
				return result, err
			}
			if indexed < eligible {
				return result, fmt.Errorf("所选模型尚未完成索引: %w", store.ErrConflict)
			}
		}
		text := q.Text
		if override, ok := q.ChannelQueries[r.channel]; ok {
			text = override
		}
		key := r.info.ID + "\x00" + text
		vec, ok := encoded[key]
		if !ok {
			response, err := p.ml.Embed(ctx, mlclient.EmbedRequest{Space: r.info.ID, Role: "query", Inputs: []mlclient.EmbedInput{{Text: text}}})
			if err != nil {
				result.Skipped = append(result.Skipped, r.channel)
				failures = append(failures, err)
				continue
			}
			vec = response.Vectors[0]
			encoded[key] = vec
		}
		hits, err := p.store.SearchVectors(ctx, store.VectorQuery{SpaceID: r.localID, Fingerprint: r.info.Fingerprint, Channel: r.channel, Seg: r.seg, Vec: vec, VideoIDs: q.VideoIDs, ProjectID: q.ProjectID, TimeRange: q.TimeRange, K: 5})
		if err != nil {
			result.Skipped = append(result.Skipped, r.channel)
			failures = append(failures, err)
			continue
		}
		list := retrieval.List{Channel: r.channel, Space: r.localID, Weight: weight, Hits: make([]retrieval.Hit, len(hits))}
		for i, hit := range hits {
			list.Hits[i] = retrieval.Hit{VideoID: hit.VideoID, Span: hit.Span, Rank: hit.Rank}
		}
		lists = append(lists, list)
	}
	if active == 0 {
		return result, store.ErrInvalid
	}
	if len(lists) == 0 && len(failures) > 0 {
		return result, errors.Join(failures...)
	}
	result.Moments = retrieval.Fuse(lists, durations, q.TimeRange, retrieval.Config{RRFK: 60, MergeGapCells: 1}, q.Limit)
	sentences := []retrieval.Sentence{}
	seen := map[int64]bool{}
	for _, m := range result.Moments {
		if seen[m.VideoID] {
			continue
		}
		seen[m.VideoID] = true
		rows, err := p.store.Sentences(ctx, m.VideoID, nil)
		if err != nil {
			return result, err
		}
		for _, s := range rows {
			sentences = append(sentences, retrieval.Sentence{VideoID: s.VideoID, Span: timeline.Span{StartMs: s.StartMs, EndMs: s.EndMs}, Text: s.Text})
		}
	}
	retrieval.AttachSnippets(result.Moments, sentences)
	slices.Sort(result.Skipped)
	return result, nil
}
