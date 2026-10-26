package search

import (
	"context"
	"maps"
	"slices"

	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/internal/workspace"
)

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

func (s *Service) defaultWeight(channel string) float64 {
	if weight, ok := s.cfg.ChannelWeights[channel]; ok {
		return weight
	}
	return 1
}

func modelCandidates(channel space.Channel, discovered []space.Info) []space.Info {
	byID := map[string]space.Info{}
	for _, id := range append(slices.Clone(channel.Active), channel.Shadow...) {
		byID[id] = space.Info{ID: id}
	}
	for _, info := range discovered {
		_, configured := byID[info.ID]
		if configured || channel.Compatible(info) {
			byID[info.ID] = info
		}
	}
	out := make([]space.Info, 0, len(byID))
	for _, id := range slices.Sorted(maps.Keys(byID)) {
		out = append(out, byID[id])
	}
	return out
}

func (s *Service) modelOption(ctx context.Context, p workspace.Principal, ids []int64, channel space.Channel, info space.Info) (ModelOption, error) {
	option := ModelOption{SpaceID: info.ID, DocInputs: append([]string{}, info.DocInputs...), QueryInputs: append([]string{}, info.QueryInputs...)}
	switch {
	case info.Fingerprint == "":
		option.Status = "unavailable"
	case !channel.Compatible(info):
		option.Status = "incompatible"
	case info.Disabled:
		option.Status = "disabled"
	case info.Endpoint == "":
		option.Status = "unavailable"
	default:
		coverage, err := index.SpaceCoverage(ctx, s.pool, p.WorkspaceID, ids, channel.Name, channel.Seg, info.ID, info.Fingerprint)
		if err != nil {
			return option, err
		}
		option.EligibleVideos, option.IndexedVideos = coverage.EligibleVideos, coverage.IndexedVideos
		ready, err := index.VectorIndexReady(ctx, s.pool, channel.Name, info.ID)
		if err != nil {
			return option, err
		}
		switch {
		case !ready:
			option.Status = "index_missing"
		case coverage.IndexedVideos < coverage.EligibleVideos:
			option.Status = "indexing"
		default:
			option.Status = "ready"
		}
	}
	return option, nil
}

func (s *Service) Models(ctx context.Context, p workspace.Principal, scope library.Scope) ([]ChannelModels, error) {
	if p.UserID <= 0 || p.WorkspaceID <= 0 {
		return nil, apperr.New(apperr.Unauthenticated, "unauthenticated", "请先登录")
	}
	ids, err := s.library.ResolveScope(ctx, p, scope)
	if err != nil {
		return nil, err
	}
	discovered := s.registry.List()
	result := make([]ChannelModels, 0)
	for _, channel := range s.channels.List() {
		defaults := append([]string{}, channel.Active...)
		slices.Sort(defaults)
		item := ChannelModels{Channel: channel.Name, DocInput: channel.Doc, DefaultSpaceIDs: defaults, DefaultWeight: s.defaultWeight(channel.Name), Models: []ModelOption{}}
		for _, info := range modelCandidates(channel, discovered) {
			option, err := s.modelOption(ctx, p, ids, channel, info)
			if err != nil {
				return nil, err
			}
			item.Models = append(item.Models, option)
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) validateOverrides(ctx context.Context, p workspace.Principal, q Query, ids []int64) error {
	if len(q.SpaceOverride) == 0 {
		return nil
	}
	discovered := s.registry.List()
	for _, name := range slices.Sorted(maps.Keys(q.SpaceOverride)) {
		channel, _ := s.channels.Get(name)
		id := q.SpaceOverride[name]
		candidates := modelCandidates(channel, discovered)
		pos := slices.IndexFunc(candidates, func(info space.Info) bool { return info.ID == id })
		if pos < 0 {
			return apperr.New(apperr.Invalid, "invalid_space_override", "搜索空间不兼容或不存在")
		}
		option, err := s.modelOption(ctx, p, ids, channel, candidates[pos])
		if err != nil {
			return err
		}
		switch option.Status {
		case "incompatible":
			return apperr.New(apperr.Invalid, "invalid_space_override", "搜索空间不兼容或不存在")
		case "unavailable", "disabled":
			return apperr.New(apperr.Unavailable, "space_unavailable", "搜索空间暂时不可用")
		case "index_missing", "indexing":
			return apperr.New(apperr.Conflict, "not_ready", "搜索空间尚未完成索引")
		}
	}
	return nil
}
