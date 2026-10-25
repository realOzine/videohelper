package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/platform/queue"
	"github.com/ozine/videohelper/internal/space"
	"github.com/redis/go-redis/v9"
)

type Commit func(context.Context, pgx.Tx) error
type RunFunc func(context.Context, *StageCtx) (Commit, error)

type StageOptions struct {
	Channel     string     `json:"channel,omitempty"`
	Seg         string     `json:"seg,omitempty"`
	Space       space.Info `json:"space,omitempty"`
	Transcriber string     `json:"transcriber,omitempty"`
	Fingerprint string     `json:"fingerprint,omitempty"`
	MaxWindowMs int        `json:"max_window_ms,omitempty"`
}

type StageDef struct {
	Name     string        `json:"name"`
	Version  string        `json:"version"`
	Queue    string        `json:"queue"`
	Deps     []string      `json:"deps"`
	Timeout  time.Duration `json:"timeout"`
	Required bool          `json:"required"`
	Options  StageOptions  `json:"options"`
}

type StageCtx struct {
	Video        library.Video
	Payload      queue.StagePayload
	Def          StageDef
	Attempt      int
	Progress     json.RawMessage
	SaveProgress func(ctx context.Context, v any, commit Commit) error
}

type Deps struct {
	Pool      *pgxpool.Pool
	Client    *asynq.Client
	Inspector *asynq.Inspector
	Redis     redis.UniversalClient
	Channels  *space.Channels
	Registry  *space.Registry
	Worker    config.WorkerConfig
	Runs      map[string]RunFunc
}

type Service struct{ deps Deps }

var ErrStale = errors.New("stage attempt is no longer current")

type stageOutput struct {
	Plan     StageDef        `json:"plan"`
	Progress json.RawMessage `json:"progress,omitempty"`
}
