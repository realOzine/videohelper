package space

import (
	"context"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"

	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/index"
)

type Channel struct {
	Name, Doc, Seg string
	Weight         float64
	Active, Shadow []string
}

type Channels struct{ channels map[string]Channel }

var identifier = regexp.MustCompile(`^[a-z0-9@._-]+$`)

func NewChannels(cfg config.ChannelsConfig) (*Channels, error) {
	channels := &Channels{channels: map[string]Channel{}}
	if len(cfg) == 0 {
		return nil, fmt.Errorf("at least one retrieval channel is required")
	}
	for name, item := range cfg {
		doc, seg := "", ""
		switch name {
		case "visual":
			doc, seg = "video", "grid@v1"
		case "speech":
			doc, seg = "text", "sentence@v1"
		default:
			return nil, fmt.Errorf("unsupported retrieval channel %q", name)
		}
		if item.Doc != doc || item.Seg != seg || item.Weight <= 0 || math.IsNaN(item.Weight) || math.IsInf(item.Weight, 0) || len(item.Active) == 0 {
			return nil, fmt.Errorf("invalid %s channel input, segmentation, weight or active spaces", name)
		}
		seen := map[string]bool{}
		for _, ids := range [][]string{item.Active, item.Shadow} {
			for _, id := range ids {
				if !identifier.MatchString(id) || seen[id] {
					return nil, fmt.Errorf("invalid or duplicated space %q in channel %s", id, name)
				}
				seen[id] = true
			}
		}
		channels.channels[name] = Channel{Name: name, Doc: item.Doc, Seg: item.Seg, Weight: item.Weight, Active: slices.Clone(item.Active), Shadow: slices.Clone(item.Shadow)}
	}
	return channels, nil
}

func cloneChannel(c Channel) Channel {
	c.Active = slices.Clone(c.Active)
	c.Shadow = slices.Clone(c.Shadow)
	return c
}

func (c *Channels) Get(name string) (Channel, bool) {
	channel, ok := c.channels[name]
	return cloneChannel(channel), ok
}

func (c *Channels) List() []Channel {
	out := make([]Channel, 0, len(c.channels))
	for _, name := range slices.Sorted(maps.Keys(c.channels)) {
		out = append(out, cloneChannel(c.channels[name]))
	}
	return out
}

func (c *Channels) Validate(ctx context.Context, r *Registry) error {
	for _, channel := range c.List() {
		if channel.Name == "speech" {
			if _, _, ok := r.Transcriber(); !ok {
				return fmt.Errorf("speech requires an available ASR model")
			}
		}
		for _, ids := range [][]string{channel.Active, channel.Shadow} {
			for _, id := range ids {
				info, ok := r.Get(id)
				if !ok || info.Disabled {
					return fmt.Errorf("channel %s space %s is unavailable or disabled", channel.Name, id)
				}
				if !slices.Contains(info.DocInputs, channel.Doc) || !slices.Contains(info.QueryInputs, "text") {
					return fmt.Errorf("channel %s space %s lacks required inputs", channel.Name, id)
				}
				if channel.Doc == "video" && info.MaxSpanMs < 2500 {
					return fmt.Errorf("space %s cannot encode grid@v1 cells", id)
				}
				ready, err := index.VectorIndexReady(ctx, r.pool, channel.Name, id)
				if err != nil {
					return err
				}
				if !ready {
					return fmt.Errorf("missing vector index for %s/%s; run space add first", channel.Name, id)
				}
			}
		}
	}
	return nil
}
