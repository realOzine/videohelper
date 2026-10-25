package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

func TestRetryAndPayload(t *testing.T) {
	throttled := fmt.Errorf("no capacity: %w", ErrThrottled)
	if IsFailure(throttled) || IsFailure(nil) || !IsFailure(errors.New("failure")) {
		t.Fatal("incorrect failure accounting")
	}
	for n, want := range []time.Duration{10 * time.Second, 20 * time.Second, 40 * time.Second, 80 * time.Second, 160 * time.Second, 320 * time.Second, 600 * time.Second} {
		if got := RetryDelay(n, errors.New("failure"), nil); got != want {
			t.Fatalf("retry %d=%v want %v", n, got, want)
		}
	}
	if d := RetryDelay(100, throttled, nil); d < 15*time.Second || d >= 20*time.Second {
		t.Fatalf("throttle delay=%v", d)
	}
	p := StagePayload{WorkspaceID: 1, VideoID: 42, Stage: "embed:visual:wemm-2b@r1", Version: "v1"}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := StageTaskID(p); got != "stage:42:embed:visual:wemm-2b@r1:v1" {
		t.Fatal(got)
	}
	p.Version = "bad:version"
	if err := p.Validate(); err == nil {
		t.Fatal("ambiguous task ID accepted")
	}
}

func TestRedisEnqueueIntegration(t *testing.T) {
	addr := os.Getenv("VH_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("set VH_TEST_REDIS_ADDR to isolated local Redis")
	}
	// A dedicated logical DB keeps this test away from running worker queues.
	r := asynq.RedisClientOpt{Addr: addr, DB: 15}
	c := asynq.NewClient(r)
	t.Cleanup(func() { c.Close() })
	i := asynq.NewInspector(r)
	t.Cleanup(func() { i.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p := StagePayload{WorkspaceID: 1, VideoID: time.Now().UnixNano(), Stage: "probe", Version: "v1"}
	id := StageTaskID(p)
	t.Cleanup(func() {
		if err := i.DeleteTask(QueueCPU, id); err != nil {
			t.Error(err)
		}
	})
	for range 2 {
		if err := EnqueueStage(ctx, c, p, QueueCPU, time.Minute, 3); err != nil {
			t.Fatal(err)
		}
	}
	info, err := i.GetTaskInfo(QueueCPU, id)
	if err != nil {
		t.Fatal(err)
	}
	var got StagePayload
	if err := json.Unmarshal(info.Payload, &got); err != nil || got != p || info.MaxRetry != 3 || info.Timeout != time.Minute || info.Retention != 0 {
		t.Fatalf("task=%+v payload=%+v err=%v", info, got, err)
	}
	now := time.Now()
	reconcileID := fmt.Sprintf("reconcile:%d", now.UTC().Truncate(time.Minute).Unix())
	t.Cleanup(func() {
		if err := i.DeleteTask(QueueMaint, reconcileID); err != nil {
			t.Error(err)
		}
	})
	for range 2 {
		if err := EnqueueReconcile(ctx, c, now, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	info, err = i.GetTaskInfo(QueueMaint, reconcileID)
	if err != nil || info.Retention != time.Hour {
		t.Fatalf("reconcile retention: %+v %v", info, err)
	}
}
