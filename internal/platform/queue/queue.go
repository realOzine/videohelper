package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"regexp"
	"time"

	"github.com/hibiken/asynq"
	"github.com/ozine/videohelper/internal/config"
)

const (
	QueueCPU      = "cpu"
	QueueGPU      = "gpu"
	QueueGPULow   = "gpu_low"
	QueueMaint    = "maint"
	TypeStage     = "stage"
	TypePurge     = "purge"
	TypeReconcile = "reconcile"
)

var ErrThrottled = errors.New("throttled")
var identifier = regexp.MustCompile(`^[a-z0-9@._-]+$`)
var stageName = regexp.MustCompile(`^[a-z0-9@._:-]+$`)

type StagePayload struct {
	WorkspaceID int64  `json:"workspace_id"`
	VideoID     int64  `json:"video_id"`
	Stage       string `json:"stage"`
	Version     string `json:"version"`
}

type PurgePayload struct {
	WorkspaceID int64 `json:"workspace_id"`
	VideoID     int64 `json:"video_id"`
}

func (p StagePayload) Validate() error {
	if p.WorkspaceID <= 0 || p.VideoID <= 0 || !stageName.MatchString(p.Stage) || !identifier.MatchString(p.Version) {
		return errors.New("stage payload requires positive workspace/video IDs and valid stage/version")
	}
	return nil
}

func StageTaskID(p StagePayload) string {
	return fmt.Sprintf("stage:%d:%s:%s", p.VideoID, p.Stage, p.Version)
}

func EnqueueStage(ctx context.Context, c *asynq.Client, p StagePayload, queue string, timeout time.Duration, maxRetry int) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if (queue != QueueCPU && queue != QueueGPU && queue != QueueGPULow) || timeout < time.Second || maxRetry < 0 {
		return errors.New("invalid stage queue, timeout or retry limit")
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = c.EnqueueContext(ctx, asynq.NewTask(TypeStage, payload), asynq.TaskID(StageTaskID(p)), asynq.Queue(queue), asynq.Timeout(timeout), asynq.MaxRetry(maxRetry))
	return deduplicated(err)
}

func EnqueuePurge(ctx context.Context, c *asynq.Client, p PurgePayload, maxRetry int) error {
	if p.WorkspaceID <= 0 || p.VideoID <= 0 || maxRetry < 0 {
		return errors.New("invalid purge payload or retry limit")
	}
	payload, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = c.EnqueueContext(ctx, asynq.NewTask(TypePurge, payload), asynq.TaskID(fmt.Sprintf("purge:%d", p.VideoID)), asynq.Queue(QueueMaint), asynq.Timeout(time.Hour), asynq.MaxRetry(maxRetry))
	return deduplicated(err)
}

// Workers call this on a ticker; the window ID deduplicates concurrent schedulers even after completion.
func EnqueueReconcile(ctx context.Context, c *asynq.Client, now time.Time, every time.Duration) error {
	if every < time.Second {
		return errors.New("reconcile interval must be at least one second")
	}
	id := fmt.Sprintf("reconcile:%d", now.UTC().Truncate(every).Unix())
	_, err := c.EnqueueContext(ctx, asynq.NewTask(TypeReconcile, []byte("{}")), asynq.TaskID(id), asynq.Queue(QueueMaint), asynq.Retention(max(time.Hour, every)), asynq.Timeout(every), asynq.MaxRetry(0))
	return deduplicated(err)
}

func deduplicated(err error) error {
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

func RetryDelay(n int, err error, _ *asynq.Task) time.Duration {
	if errors.Is(err, ErrThrottled) {
		return 15*time.Second + time.Duration(rand.IntN(5000))*time.Millisecond
	}
	return min(10*time.Second<<min(max(n, 0), 6), 10*time.Minute)
}

func IsFailure(err error) bool { return err != nil && !errors.Is(err, ErrThrottled) }

func NewServers(cfg config.WorkerConfig, r asynq.RedisConnOpt) (cpu, gpu *asynq.Server) {
	makeServer := func(concurrency int, queues map[string]int) *asynq.Server {
		return asynq.NewServer(r, asynq.Config{Concurrency: concurrency, Queues: queues, RetryDelayFunc: RetryDelay, IsFailure: IsFailure})
	}
	return makeServer(cfg.CPUConcurrency, map[string]int{QueueCPU: 9, QueueMaint: 1}), makeServer(cfg.GPUConcurrency, map[string]int{QueueGPU: 9, QueueGPULow: 1})
}
