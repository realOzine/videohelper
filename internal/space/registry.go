package space

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/space/spacedb"
)

type Info struct {
	ID, Metric, Fingerprint  string
	Dim, MaxSpanMs, MaxBatch int
	DocInputs, QueryInputs   []string
	Endpoint                 string
	Disabled                 bool
}

type transcriber struct {
	info     mlclient.TranscriberInfo
	endpoint string
}

type Registry struct {
	selectedASR  string
	bindings     map[string]string
	pool         *pgxpool.Pool
	ml           *mlclient.Client
	syncMu       sync.Mutex
	mu           sync.RWMutex
	spaces       map[string]Info
	transcribers map[string]transcriber
}

func New(pool *pgxpool.Pool, client *mlclient.Client, cfg config.MLConfig) *Registry {
	return &Registry{selectedASR: cfg.Transcriber, bindings: maps.Clone(cfg.ModelEndpoints), pool: pool, ml: client, spaces: map[string]Info{}, transcribers: map[string]transcriber{}}
}

// Sync discovers outside the transaction and preserves persisted identity on fingerprint drift.
func (r *Registry) Sync(ctx context.Context) error {
	r.syncMu.Lock()
	defer r.syncMu.Unlock()
	discovery := r.ml.Discover(ctx)
	spaces, transcribers, conflicts := collect(discovery.Endpoints, r.bindings)
	err := db.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		queries := spacedb.New(tx)
		for _, id := range slices.Sorted(maps.Keys(spaces)) {
			info := spaces[id]
			if info.Dim < 1 || info.Dim > 4000 || info.MaxSpanMs > math.MaxInt32 {
				return fmt.Errorf("space %s exceeds database dimensions or time range", id)
			}
			span := pgtype.Int4{Int32: int32(info.MaxSpanMs), Valid: info.MaxSpanMs > 0}
			disabled, err := queries.RegisterSpace(ctx, spacedb.RegisterSpaceParams{ID: info.ID, Dim: int32(info.Dim), Metric: info.Metric, Fingerprint: info.Fingerprint, DocInputs: info.DocInputs, QueryInputs: info.QueryInputs, MaxSpanMs: span})
			if err != nil {
				return err
			}
			if conflicts[id] {
				if err := queries.DisableSpace(ctx, id); err != nil {
					return err
				}
				disabled = true
			}
			info.Disabled = disabled
			spaces[id] = info
			if disabled {
				slog.Error("embedding space disabled: identity mismatch or operator action", "space", id)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.spaces, r.transcribers = spaces, transcribers
	r.mu.Unlock()
	var endpointErrors []error
	for endpoint, err := range discovery.Errors {
		slog.Warn("ML endpoint discovery failed", "endpoint", endpoint, "error", err)
		endpointErrors = append(endpointErrors, fmt.Errorf("discover %s: %w", endpoint, err))
	}
	if len(discovery.Endpoints) == 0 {
		return errors.Join(append(endpointErrors, errors.New("no ML endpoint available"))...)
	}
	return nil
}

func collect(endpoints map[string]mlclient.SpacesResponse, bindings map[string]string) (map[string]Info, map[string]transcriber, map[string]bool) {
	spaces := map[string]Info{}
	transcribers := map[string]transcriber{}
	conflicts := map[string]bool{}
	transcriberConflicts := map[string]bool{}
	for _, endpoint := range slices.Sorted(maps.Keys(endpoints)) {
		metadata := endpoints[endpoint]
		for _, item := range metadata.Spaces {
			info := Info{ID: item.ID, Dim: item.Dim, Metric: item.Metric, Fingerprint: item.Fingerprint, DocInputs: slices.Clone(item.DocInputs), QueryInputs: slices.Clone(item.QueryInputs), MaxBatch: item.MaxBatch, Endpoint: endpoint}
			if item.MaxSpanMs != nil {
				info.MaxSpanMs = *item.MaxSpanMs
			}
			if previous, ok := spaces[item.ID]; ok {
				if previous.Fingerprint != info.Fingerprint || previous.Dim != info.Dim || previous.Metric != info.Metric {
					conflicts[item.ID] = true
				}
				if bindings[item.ID] == endpoint {
					spaces[item.ID] = info
				} else if bindings[item.ID] == "" {
					previous.Endpoint = ""
					spaces[item.ID] = previous
				}
				continue
			}
			if selected := bindings[item.ID]; selected != "" && selected != endpoint {
				info.Endpoint = ""
			}
			spaces[item.ID] = info
		}
		for _, item := range metadata.Transcribers {
			if previous, ok := transcribers[item.ID]; ok {
				if previous.info.Fingerprint != item.Fingerprint {
					transcriberConflicts[item.ID] = true
				}
				if bindings[item.ID] == endpoint {
					item.Languages = slices.Clone(item.Languages)
					transcribers[item.ID] = transcriber{info: item, endpoint: endpoint}
				} else if bindings[item.ID] == "" {
					previous.endpoint = ""
					transcribers[item.ID] = previous
				}
				continue
			}
			item.Languages = slices.Clone(item.Languages)
			provider := endpoint
			if selected := bindings[item.ID]; selected != "" && selected != endpoint {
				provider = ""
			}
			transcribers[item.ID] = transcriber{info: item, endpoint: provider}
		}
	}
	for id, model := range transcribers {
		if transcriberConflicts[id] || model.endpoint == "" {
			delete(transcribers, id)
		}
	}
	return spaces, transcribers, conflicts
}

func (r *Registry) Get(id string) (Info, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.spaces[id]
	info.DocInputs = slices.Clone(info.DocInputs)
	info.QueryInputs = slices.Clone(info.QueryInputs)
	return info, ok && info.Endpoint != ""
}

// List includes unroutable and disabled spaces so callers can report their status.
func (r *Registry) List() []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Info, 0, len(r.spaces))
	for _, id := range slices.Sorted(maps.Keys(r.spaces)) {
		info := r.spaces[id]
		info.DocInputs = slices.Clone(info.DocInputs)
		info.QueryInputs = slices.Clone(info.QueryInputs)
		out = append(out, info)
	}
	return out
}

func (r *Registry) Resolve(id string) (string, bool) {
	info, ok := r.Get(id)
	return info.Endpoint, ok && !info.Disabled
}

func (r *Registry) Transcriber() (mlclient.TranscriberInfo, string, bool) {
	return r.GetTranscriber(r.selectedASR)
}

func (r *Registry) GetTranscriber(id string) (mlclient.TranscriberInfo, string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	selected, ok := r.transcribers[id]
	selected.info.Languages = slices.Clone(selected.info.Languages)
	return selected.info, selected.endpoint, ok
}
