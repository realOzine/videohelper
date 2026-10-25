package pipeline

import (
	"fmt"
	"time"

	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/platform/queue"
	"github.com/ozine/videohelper/internal/space"
)

func InitialPlan(_ library.Video) []StageDef {
	return []StageDef{{Name: "probe", Version: "v1", Queue: queue.QueueCPU, Timeout: 5 * time.Minute, Required: true}}
}

func ExpandPlan(v library.Video, channels []space.Channel, registry *space.Registry) ([]StageDef, error) {
	if v.HasVideo == nil || v.HasAudio == nil {
		return nil, fmt.Errorf("probe metadata is required before expanding the stage plan")
	}
	plan := []StageDef{{Name: "prepare_media", Version: "v1", Queue: queue.QueueCPU, Deps: []string{"probe"}, Timeout: 3 * time.Hour, Required: true}}
	finalDeps := []string{"prepare_media"}
	if *v.HasAudio {
		model, _, ok := registry.Transcriber()
		if !ok {
			return nil, fmt.Errorf("ASR model unavailable")
		}
		plan = append(plan, StageDef{Name: "asr", Version: "v1", Queue: queue.QueueGPU, Deps: []string{"prepare_media"}, Timeout: 2 * time.Hour, Required: true,
			Options: StageOptions{Transcriber: model.ID, Fingerprint: model.Fingerprint, MaxWindowMs: model.MaxWindowMs}})
		finalDeps = append(finalDeps, "asr")
	}
	for _, channel := range channels {
		eligible := channel.Name == "visual" && *v.HasVideo || (channel.Name == "speech" || channel.Name == "audio") && *v.HasAudio
		if !eligible {
			continue
		}
		dep, timeout := "prepare_media", 3*time.Hour
		if channel.Name == "speech" {
			dep, timeout = "asr", time.Hour
		}
		for _, group := range []struct {
			ids      []string
			required bool
		}{{channel.Active, true}, {channel.Shadow, false}} {
			for _, id := range group.ids {
				info, ok := registry.Get(id)
				if !ok || info.Disabled {
					return nil, fmt.Errorf("embedding space %s unavailable", id)
				}
				name := "embed:" + channel.Name + ":" + id
				q := queue.QueueGPU
				if !group.required {
					q = queue.QueueGPULow
				}
				plan = append(plan, StageDef{Name: name, Version: "v1", Queue: q, Deps: []string{dep}, Timeout: timeout, Required: group.required,
					Options: StageOptions{Channel: channel.Name, Seg: channel.Seg, Space: info}})
				if group.required {
					finalDeps = append(finalDeps, name)
				}
			}
		}
	}
	return append(plan, StageDef{Name: "finalize", Version: "v1", Queue: queue.QueueCPU, Deps: finalDeps, Timeout: time.Minute, Required: true}), nil
}
