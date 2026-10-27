package processor

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ozine/videohelper/internal/config"
	desktopruntime "github.com/ozine/videohelper/internal/desktop/runtime"
	"github.com/ozine/videohelper/internal/desktop/store"
	"github.com/ozine/videohelper/internal/media"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/timeline"
)

type route struct {
	channel, doc, seg, localID string
	info                       mlclient.SpaceInfo
	weight                     float64
}
type Processor struct {
	store       *store.Store
	cfg         Config
	ml          *mlclient.Client
	media       media.Tools
	routes      []route
	asr         mlclient.TranscriberInfo
	asrEndpoint string
	identity    map[string]string
	wake        chan struct{}
	mu          sync.Mutex
	active      map[int64]context.CancelFunc
}

type checkpoint struct {
	Identity map[string]string      `json:"identity"`
	Done     map[string]bool        `json:"done"`
	Next     map[string]int         `json:"next"`
	RawKeys  []string               `json:"raw_keys,omitempty"`
	Stages   map[string]StageStatus `json:"stages"`
}

func New(ctx context.Context, s *store.Store, cfg Config) (*Processor, error) {
	if s == nil || len(cfg.Endpoints) == 0 || cfg.VisualSpace == "" || cfg.AudioSpace == "" || cfg.Transcriber == "" {
		return nil, store.ErrInvalid
	}
	p := &Processor{store: s, cfg: cfg, wake: make(chan struct{}, 1), active: map[int64]context.CancelFunc{}, identity: map[string]string{}}
	p.media = media.Tools{FFmpeg: cfg.Tools.FFmpeg, FFprobe: cfg.Tools.FFprobe, RunCommand: desktopruntime.RunCommand}
	endpoints := map[string]string{}
	p.ml = mlclient.New(config.MLConfig{Endpoints: cfg.Endpoints, Token: cfg.Token, QueryTimeout: 2 * time.Minute, EmbedTimeout: 3 * time.Hour, TranscribeTimeout: 2 * time.Hour}, func(id string) (string, bool) { e, ok := endpoints[id]; return e, ok })
	spaces := map[string]mlclient.SpaceInfo{}
	discovery := p.ml.Discover(ctx)
	for _, endpoint := range slices.Sorted(maps.Keys(discovery.Endpoints)) {
		found := discovery.Endpoints[endpoint]
		for _, sp := range found.Spaces {
			if old, ok := spaces[sp.ID]; ok && (old.Fingerprint != sp.Fingerprint || old.Dim != sp.Dim || old.Metric != sp.Metric) {
				return nil, fmt.Errorf("conflicting ML space %s", sp.ID)
			}
			spaces[sp.ID] = sp
			endpoints[sp.ID] = endpoint
		}
		for _, asr := range found.Transcribers {
			if asr.ID == cfg.Transcriber {
				if p.asr.ID != "" && p.asr.Fingerprint != asr.Fingerprint {
					return nil, store.ErrConflict
				}
				p.asr, p.asrEndpoint = asr, endpoint
			}
		}
	}
	if p.asr.ID == "" {
		return nil, fmt.Errorf("transcriber %s unavailable: %v", cfg.Transcriber, discovery.Errors)
	}
	p.identity["asr"] = p.asr.Fingerprint
	for _, spec := range []struct {
		channel, doc, id, seg string
		weight                float64
	}{{"visual", "video", cfg.VisualSpace, timeline.GridSeg, 1}, {"speech", "text", cfg.VisualSpace, timeline.SentenceSeg, 1}, {"audio", "audio", cfg.AudioSpace, timeline.GridSeg, 0}} {
		sp, ok := spaces[spec.id]
		if !ok || !slices.Contains(sp.DocInputs, spec.doc) || !slices.Contains(sp.QueryInputs, "text") {
			return nil, fmt.Errorf("space %s cannot encode %s", spec.id, spec.channel)
		}
		identity := sp.Fingerprint
		if spec.channel == "speech" {
			identity += "/" + p.asr.Fingerprint
		}
		sum := sha256.Sum256([]byte(identity))
		localID := sp.ID + "." + fmt.Sprintf("%x", sum[:])
		if _, err := s.RegisterSpace(ctx, store.Space{ID: localID, Fingerprint: sp.Fingerprint, Dim: sp.Dim, Metric: sp.Metric, DocInputs: sp.DocInputs, QueryInputs: sp.QueryInputs}); err != nil {
			return nil, err
		}
		p.routes = append(p.routes, route{channel: spec.channel, doc: spec.doc, seg: spec.seg, localID: localID, info: sp, weight: spec.weight})
		p.identity[spec.channel] = localID
	}
	return p, nil
}

func (p *Processor) notify() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}
func (p *Processor) ImportFile(ctx context.Context, path string, projectID *int64) (store.Video, error) {
	v, err := p.store.ImportFile(ctx, path, projectID)
	if err != nil {
		return v, err
	}
	p.notify()
	return v, nil
}
func (p *Processor) ImportURL(ctx context.Context, raw string, projectID *int64) (store.Video, error) {
	v, err := p.store.ImportURL(ctx, raw, projectID)
	if err != nil {
		return v, err
	}
	p.notify()
	return v, nil
}

func (p *Processor) Cancel(ctx context.Context, videoID int64) error {
	j, err := p.store.LatestJob(ctx, videoID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if j.Status != "queued" && j.Status != "running" {
		return nil
	}
	if err := p.store.CancelJob(ctx, j.ID); err != nil && !errors.Is(err, store.ErrConflict) {
		return err
	}
	p.mu.Lock()
	cancel := p.active[j.ID]
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (p *Processor) Retry(ctx context.Context, videoID int64) (store.Job, error) {
	j, err := p.store.LatestJob(ctx, videoID)
	var old checkpoint
	if err == nil {
		if e := json.Unmarshal(j.Progress, &old); e != nil {
			return store.Job{}, e
		}
	}
	if errors.Is(err, store.ErrNotFound) || err == nil && (j.Status == "done" || !maps.Equal(old.Identity, p.identity)) {
		j, err = p.store.CreateJob(ctx, videoID)
	} else if err == nil {
		j, err = p.store.RetryJob(ctx, j.ID)
	}
	if err == nil {
		p.notify()
	}
	return j, err
}

// Run owns the only scheduler. Its context belongs to the application, not a
// browser request or window. Unfinished jobs retain their durable checkpoints.
func (p *Processor) Run(ctx context.Context) error {
	if _, err := p.store.RecoverJobs(ctx); err != nil {
		return err
	}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		j, err := p.store.ClaimJob(ctx)
		if errors.Is(err, store.ErrNotFound) {
			select {
			case <-ctx.Done():
				return nil
			case <-p.wake:
			case <-tick.C:
			}
			continue
		}
		if err != nil {
			return err
		}
		jobCtx, cancel := context.WithCancel(ctx)
		p.mu.Lock()
		p.active[j.ID] = cancel
		p.mu.Unlock()
		err = p.process(jobCtx, j)
		cancel()
		p.mu.Lock()
		delete(p.active, j.ID)
		p.mu.Unlock()
		if ctx.Err() != nil {
			return nil
		}
		if err != nil && !errors.Is(err, store.ErrStale) {
			cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			failErr := p.store.FailJob(cleanup, j.ID, j.Attempt, err.Error())
			stop()
			if failErr != nil && !errors.Is(failErr, store.ErrStale) {
				return errors.Join(err, failErr)
			}
		}
	}
}

func (p *Processor) Stages(ctx context.Context, videoID int64) ([]StageStatus, error) {
	j, err := p.store.LatestJob(ctx, videoID)
	if errors.Is(err, store.ErrNotFound) {
		return []StageStatus{}, nil
	}
	if err != nil {
		return nil, err
	}
	var cp checkpoint
	if err := json.Unmarshal(j.Progress, &cp); err != nil {
		return nil, err
	}
	stages := make([]StageStatus, 0, len(cp.Stages))
	for _, stage := range cp.Stages {
		if stage.Stage == j.Stage && (j.Status == "failed" || j.Status == "cancelled") {
			stage.Status = j.Status
			stage.Error = j.Error
			stage.UpdatedAt = j.UpdatedAt
		}
		stages = append(stages, stage)
	}
	slices.SortFunc(stages, func(a, b StageStatus) int { return strings.Compare(a.Stage, b.Stage) })
	return stages, nil
}

func (p *Processor) save(ctx context.Context, j store.Job, cp *checkpoint, stage, status string, next, total int, output func(*store.Tx) error) error {
	var progress *float64
	if total > 0 {
		f := float64(next) / float64(total)
		progress = &f
	}
	cp.Stages[stage] = StageStatus{Stage: stage, Version: "desktop-v1", Status: status, Attempt: j.Attempt, Required: true, Progress: progress, UpdatedAt: time.Now().UTC()}
	if status == "done" {
		cp.Done[stage] = true
	}
	return p.store.Checkpoint(ctx, j.ID, j.Attempt, stage, cp, output)
}
