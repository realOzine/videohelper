package pipeline

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/pipeline/pipelinedb"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/platform/queue"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/internal/timeline"
)

type backfillTarget struct {
	channel            space.Channel
	spaceID, name, dep string
	timeout            time.Duration
}

func (s *Service) backfillTargets() []backfillTarget {
	var targets []backfillTarget
	for _, channel := range s.deps.Channels.List() {
		dep, timeout := "prepare_media", 3*time.Hour
		if channel.Name == "speech" {
			dep, timeout = "asr", time.Hour
		}
		for _, id := range append(slices.Clone(channel.Active), channel.Shadow...) {
			info, ok := s.deps.Registry.Get(id)
			if !ok || info.Disabled || !slices.Contains(info.DocInputs, channel.Doc) {
				continue
			}
			targets = append(targets, backfillTarget{channel: channel, spaceID: id, name: "embed:" + channel.Name + ":" + id, dep: dep, timeout: timeout})
		}
	}
	return targets
}

func (s *Service) backfill(ctx context.Context) error {
	targets := s.backfillTargets()
	if len(targets) == 0 {
		return nil
	}
	params := pipelinedb.BackfillVideosParams{}
	for _, target := range targets {
		params.Stages = append(params.Stages, target.name)
		params.Channels = append(params.Channels, target.channel.Name)
		params.Deps = append(params.Deps, target.dep)
	}
	ids, err := pipelinedb.New(s.deps.Pool).BackfillVideos(ctx, params)
	if err != nil {
		return err
	}
	var failures []error
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(failures, err)...)
		}
		if err := s.backfillVideo(ctx, id, targets); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *Service) backfillVideo(ctx context.Context, id int64, targets []backfillTarget) error {
	var tasks []stageTask
	err := db.WithTx(ctx, s.deps.Pool, func(tx pgx.Tx) error {
		video, err := library.GetForWorker(ctx, tx, id)
		if apperr.KindOf(err) == apperr.NotFound {
			return nil
		}
		if err != nil {
			return err
		}
		if video.Status != "ready" || video.NormalizedVideoKey == "" || video.DurationMs == nil {
			return nil
		}
		q := pipelinedb.New(tx)
		rows, err := q.Stages(ctx, id)
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, row := range rows {
			seen[row.Stage] = true
		}
		var plan []StageDef
		for _, target := range targets {
			if seen[target.name] || !dependenciesDone([]string{target.dep}, rows) {
				continue
			}
			hasMedia := video.HasAudio
			if target.channel.Name == "visual" {
				hasMedia = video.HasVideo
			}
			if hasMedia == nil || !*hasMedia {
				continue
			}
			info, ok := s.deps.Registry.Get(target.spaceID)
			if !ok || info.Disabled || !slices.Contains(info.DocInputs, target.channel.Doc) {
				continue
			}
			if target.channel.Doc != "text" && info.MaxSpanMs < timeline.CellMs(*video.DurationMs) {
				continue
			}
			_, err = q.BackfillSpace(ctx, pipelinedb.BackfillSpaceParams{ID: info.ID, Fingerprint: info.Fingerprint, Dim: int32(info.Dim), Metric: info.Metric})
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			if err != nil {
				return err
			}
			plan = append(plan, StageDef{Name: target.name, Version: "v1", Queue: queue.QueueGPULow, Deps: []string{target.dep}, Timeout: target.timeout, Required: false,
				Options: StageOptions{Channel: target.channel.Name, Seg: target.channel.Seg, Space: info}})
		}
		if len(plan) == 0 {
			return nil
		}
		if err := addPlan(ctx, tx, video, plan); err != nil {
			return err
		}
		tasks, err = advance(ctx, tx, video)
		tasks = slices.DeleteFunc(tasks, func(task stageTask) bool {
			return !slices.ContainsFunc(plan, func(def StageDef) bool { return def.Name == task.payload.Stage })
		})
		return err
	})
	if err != nil {
		return err
	}
	return s.enqueue(ctx, tasks)
}
