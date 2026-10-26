package search

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/internal/workspace"
)

type Service struct {
	pool     *pgxpool.Pool
	library  *library.Service
	registry *space.Registry
	channels *space.Channels
	ml       *mlclient.Client
	cfg      config.SearchConfig
}

func New(pool *pgxpool.Pool, library *library.Service, registry *space.Registry, channels *space.Channels, ml *mlclient.Client, cfg config.SearchConfig) *Service {
	return &Service{pool: pool, library: library, registry: registry, channels: channels, ml: ml, cfg: cfg}
}

func (s *Service) validate(p workspace.Principal, q *Query) error {
	if p.UserID <= 0 || p.WorkspaceID <= 0 {
		return apperr.New(apperr.Unauthenticated, "unauthenticated", "请先登录")
	}
	if q.Limit == 0 {
		q.Limit = min(20, s.cfg.MaxLimit)
	}
	if strings.TrimSpace(q.Text) == "" || q.Limit < 1 || q.Limit > s.cfg.MaxLimit {
		return apperr.New(apperr.Invalid, "invalid_query", "查询文字或返回数量不合法")
	}
	if q.TimeRange != nil && (q.TimeRange.StartMs < 0 || q.TimeRange.EndMs <= q.TimeRange.StartMs || q.TimeRange.EndMs > math.MaxInt32) {
		return apperr.New(apperr.Invalid, "invalid_time_range", "搜索时间范围不合法")
	}
	for _, channel := range q.OnlyChannels {
		if _, ok := s.channels.Get(channel); !ok {
			return apperr.New(apperr.Invalid, "invalid_channel", "搜索通道不合法")
		}
	}
	for channel, weight := range q.ChannelWeights {
		configured, ok := s.channels.Get(channel)
		if !ok || weight < 0 || math.IsNaN(weight) || math.IsInf(weight, 0) || math.IsInf(weight*configured.Weight, 0) {
			return apperr.New(apperr.Invalid, "invalid_channel_weight", "搜索通道权重不合法")
		}
	}
	for channel, text := range q.ChannelQueries {
		if _, ok := s.channels.Get(channel); !ok || strings.TrimSpace(text) == "" {
			return apperr.New(apperr.Invalid, "invalid_channel_query", "搜索通道查询不合法")
		}
	}
	for channel, id := range q.SpaceOverride {
		if _, ok := s.channels.Get(channel); !ok || id == "" {
			return apperr.New(apperr.Invalid, "invalid_space_override", "搜索空间覆盖不合法")
		}
	}
	return nil
}

type route struct {
	channel  space.Channel
	info     space.Info
	text     string
	weight   float64
	encoding int
}

func (s *Service) routes(q Query) ([]route, map[string]bool, []error) {
	routes := make([]route, 0)
	skipped := map[string]bool{}
	var failures []error
	for _, channel := range s.channels.List() {
		if len(q.OnlyChannels) > 0 && !slices.Contains(q.OnlyChannels, channel.Name) {
			continue
		}
		multiplier := 1.0
		if weight, ok := s.cfg.ChannelWeights[channel.Name]; ok {
			multiplier = weight
		}
		if override, ok := q.ChannelWeights[channel.Name]; ok {
			multiplier = override
		}
		weight := channel.Weight * multiplier
		if weight == 0 {
			continue
		}
		text := q.Text
		if override, ok := q.ChannelQueries[channel.Name]; ok {
			text = override
		}
		ids := channel.Active
		if override, ok := q.SpaceOverride[channel.Name]; ok {
			ids = []string{override}
		}
		ids = slices.Clone(ids)
		slices.Sort(ids)
		for _, id := range ids {
			info, ok := s.registry.Get(id)
			if !ok || info.Disabled || !slices.Contains(info.DocInputs, channel.Doc) || !slices.Contains(info.QueryInputs, "text") {
				skipped[channel.Name] = true
				failures = append(failures, fmt.Errorf("channel %s space %s unavailable", channel.Name, id))
				continue
			}
			routes = append(routes, route{channel: channel, info: info, text: text, weight: weight})
		}
	}
	return routes, skipped, failures
}

type encodingKey struct{ space, text string }
type encoding struct {
	key      encodingKey
	response mlclient.EmbedResponse
	err      error
}

func (s *Service) encode(ctx context.Context, routes []route) []encoding {
	tasks := make([]encoding, 0)
	byKey := map[encodingKey]int{}
	for i := range routes {
		key := encodingKey{space: routes[i].info.ID, text: routes[i].text}
		id, ok := byKey[key]
		if !ok {
			id = len(tasks)
			byKey[key] = id
			tasks = append(tasks, encoding{key: key})
		}
		routes[i].encoding = id
	}
	var wg sync.WaitGroup
	gate := make(chan struct{}, 4)
	for i := range tasks {
		wg.Go(func() {
			select {
			case gate <- struct{}{}:
				defer func() { <-gate }()
			case <-ctx.Done():
				tasks[i].err = ctx.Err()
				return
			}
			task := &tasks[i]
			task.response, task.err = s.ml.Embed(ctx, mlclient.EmbedRequest{Space: task.key.space, Role: "query", Inputs: []mlclient.EmbedInput{{Text: task.key.text}}})
		})
	}
	wg.Wait()
	return tasks
}

type recalled struct {
	list recallList
	err  error
}

func (s *Service) recall(ctx context.Context, p workspace.Principal, q Query, ids []int64, routes []route, encoded []encoding) []recalled {
	results := make([]recalled, len(routes))
	var wg sync.WaitGroup
	gate := make(chan struct{}, 4)
	for i, r := range routes {
		wg.Go(func() {
			result := &results[i]
			result.list = recallList{channel: r.channel.Name, space: r.info.ID, weight: r.weight}
			task := encoded[r.encoding]
			if task.err != nil {
				result.err = task.err
				return
			}
			response := task.response
			current, ok := s.registry.Get(r.info.ID)
			if !ok || current.Disabled || current.Fingerprint != r.info.Fingerprint || current.Dim != r.info.Dim || current.Metric != r.info.Metric || response.Space != r.info.ID || response.Fingerprint != r.info.Fingerprint || response.Dim != r.info.Dim || len(response.Vectors) != 1 || len(response.Vectors[0]) != r.info.Dim {
				result.err = mlclient.ErrContract
				return
			}
			select {
			case gate <- struct{}{}:
				defer func() { <-gate }()
			case <-ctx.Done():
				result.err = ctx.Err()
				return
			}
			result.list.hits, result.err = index.SearchVectors(ctx, s.pool, index.VectorQuery{WorkspaceID: p.WorkspaceID, VideoIDs: ids, Channel: r.channel.Name, Seg: r.channel.Seg, Space: index.VectorSpace{ID: r.info.ID, Dim: r.info.Dim, Metric: r.info.Metric}, Vec: response.Vectors[0], TimeRange: q.TimeRange, K: s.cfg.K})
		})
	}
	wg.Wait()
	return results
}

func (s *Service) Search(ctx context.Context, p workspace.Principal, q Query) (Result, error) {
	result := Result{Moments: []Moment{}, Skipped: []string{}}
	if err := s.validate(p, &q); err != nil {
		return result, err
	}
	ids, err := s.library.ResolveScope(ctx, p, q.Scope)
	if err != nil {
		return result, err
	}
	if ids != nil && len(ids) == 0 {
		return result, nil
	}
	routes, skipped, failures := s.routes(q)
	encoded := s.encode(ctx, routes)
	if err := ctx.Err(); err != nil {
		return result, err
	}
	recalled := s.recall(ctx, p, q, ids, routes, encoded)
	if err := ctx.Err(); err != nil {
		return result, err
	}
	lists := make([]recallList, 0, len(recalled))
	for _, response := range recalled {
		if response.err != nil {
			skipped[response.list.channel] = true
			failures = append(failures, response.err)
			slog.WarnContext(ctx, "search route skipped", "channel", response.list.channel, "space", response.list.space, "error", response.err)
		} else {
			lists = append(lists, response.list)
		}
	}
	result.Skipped = slices.Sorted(maps.Keys(skipped))
	if len(lists) == 0 && len(failures) > 0 {
		return result, apperr.Wrap(apperr.Unavailable, "search_unavailable", errors.Join(failures...))
	}
	videoIDs := map[int64]bool{}
	for _, list := range lists {
		for _, hit := range list.hits {
			videoIDs[hit.VideoID] = true
		}
	}
	if len(videoIDs) == 0 {
		return result, nil
	}
	durations, err := s.library.Durations(ctx, p.WorkspaceID, slices.Sorted(maps.Keys(videoIDs)))
	if err != nil {
		return result, err
	}
	result.Moments = fuse(lists, durations, q.TimeRange, s.cfg, q.Limit)
	ranges := make([]index.VideoSpan, 0, len(result.Moments))
	for _, moment := range result.Moments {
		ranges = append(ranges, index.VideoSpan{VideoID: moment.VideoID, Span: moment.Span})
	}
	sentences, err := index.TextRanges(ctx, s.pool, p.WorkspaceID, ranges)
	if err != nil {
		return Result{}, err
	}
	attachSnippets(result.Moments, sentences)
	return result, nil
}
