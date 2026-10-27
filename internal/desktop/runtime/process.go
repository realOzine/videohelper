package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"
)

type tailBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if n >= 8192 {
		b.data = append(b.data[:0], p[n-8192:]...)
	} else {
		if len(b.data)+n > 8192 {
			b.data = b.data[len(b.data)+n-8192:]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}
func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(bytes.Clone(b.data))
}

type ownedProcess struct {
	cmd     *exec.Cmd
	done    chan struct{}
	err     error
	cleanup func()
}

func startOwned(ctx context.Context, cmd *exec.Cmd) (*ownedProcess, error) {
	afterStart, cleanup, err := setupProcess(cmd)
	if err != nil {
		return nil, err
	}
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		cleanup()
		return nil, err
	}
	if err := afterStart(); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		cleanup()
		return nil, err
	}
	p := &ownedProcess{cmd: cmd, done: make(chan struct{}), cleanup: cleanup}
	go func() { p.err = cmd.Wait(); cleanup(); close(p.done) }()
	return p, nil
}

// RunCommand cancels only this command's process tree, including its FFmpeg/JS children.
func RunCommand(ctx context.Context, path string, args []string, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	p, err := startOwned(ctx, cmd)
	if err != nil {
		return err
	}
	<-p.done
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return p.err
}

func (p *ownedProcess) stop(ctx context.Context) error {
	select {
	case <-p.done:
		return nil
	default:
	}
	if err := p.cmd.Cancel(); err != nil && !errors.Is(err, exec.ErrNotFound) { // Wait still reaps an already exited child.
		select {
		case <-p.done:
			return nil
		default:
		}
	}
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
