package stages

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/pipeline"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/space"
)

type Config struct {
	Media      config.MediaConfig
	ML         config.MLConfig
	PresignTTL time.Duration
}

type Handlers struct {
	pool     *pgxpool.Pool
	objects  *blob.Store
	ml       *mlclient.Client
	registry *space.Registry
	cfg      Config
}

func New(pool *pgxpool.Pool, objects *blob.Store, ml *mlclient.Client, registry *space.Registry, cfg Config) *Handlers {
	return &Handlers{pool: pool, objects: objects, ml: ml, registry: registry, cfg: cfg}
}

func (h *Handlers) Runs() map[string]pipeline.RunFunc {
	return map[string]pipeline.RunFunc{"probe": h.Probe, "prepare_media": h.PrepareMedia, "finalize": h.Finalize, "asr": h.ASR, "embed": h.Embed}
}
