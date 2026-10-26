package search

import (
	"maps"
	"slices"
	"testing"

	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/internal/workspace"
)

func TestDefaultChannelWeightRouting(t *testing.T) {
	channels, err := space.NewChannels(config.ChannelsConfig{
		"visual": {Doc: "video", Seg: "grid@v1", Weight: 1, Active: []string{"wemm-2b@r1"}},
		"speech": {Doc: "text", Seg: "sentence@v1", Weight: 1, Active: []string{"wemm-2b@r1"}},
		"audio":  {Doc: "audio", Seg: "grid@v1", Weight: 1, Active: []string{"clap-htsat-unfused@r1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defaults := map[string]float64{"visual": 1, "speech": 1, "audio": 0}
	originalDefaults := maps.Clone(defaults)
	service := New(nil, nil, space.New(nil, nil, config.MLConfig{}), channels, nil, config.SearchConfig{MaxLimit: 50, ChannelWeights: defaults})
	for _, test := range []struct {
		name  string
		query Query
		want  []string
	}{
		{name: "ordinary request", want: []string{"speech", "visual"}},
		{name: "explicit winning weights", query: Query{ChannelWeights: map[string]float64{"visual": 1, "speech": 1, "audio": 0}}, want: []string{"speech", "visual"}},
		{name: "audio override restores route", query: Query{ChannelWeights: map[string]float64{"audio": 1}}, want: []string{"audio", "speech", "visual"}},
		{name: "partial inherits audio zero", query: Query{ChannelWeights: map[string]float64{"visual": 0}}, want: []string{"speech"}},
		{name: "only channels still uses defaults", query: Query{OnlyChannels: []string{"audio"}}},
		{name: "audio only explicit restore", query: Query{OnlyChannels: []string{"audio"}, ChannelWeights: map[string]float64{"audio": 1}}, want: []string{"audio"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := test.query
			query.Text = "原始中文查询"
			originalQuery := maps.Clone(query.ChannelWeights)
			if err := service.validate(workspace.Principal{UserID: 1, WorkspaceID: 1}, &query); err != nil {
				t.Fatal(err)
			}
			// An empty registry records each enabled route as unavailable, without model or DB calls.
			routes, skipped, failures := service.routes(query)
			if len(routes) != 0 || !slices.Equal(slices.Sorted(maps.Keys(skipped)), test.want) || len(failures) != len(test.want) {
				t.Fatalf("routes=%v skipped=%v failures=%v; want attempts for %v", routes, skipped, failures, test.want)
			}
			if !maps.Equal(query.ChannelWeights, originalQuery) || !maps.Equal(defaults, originalDefaults) {
				t.Fatalf("routing mutated request or configuration weights: query=%v defaults=%v", query.ChannelWeights, defaults)
			}
		})
	}
	service.cfg.ChannelWeights = nil
	_, skipped, failures := service.routes(Query{Text: "旧配置"})
	if !slices.Equal(slices.Sorted(maps.Keys(skipped)), []string{"audio", "speech", "visual"}) || len(failures) != 3 {
		t.Fatalf("omitted defaults must preserve all prior routes: skipped=%v failures=%v", skipped, failures)
	}
}
