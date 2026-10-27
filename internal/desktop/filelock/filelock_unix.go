//go:build !windows

package filelock

import (
	"errors"
	"io"
	"os"
	"sync"
	"syscall"
)

type lock struct {
	file *os.File
	once sync.Once
	err  error
}

func (l *lock) Close() error {
	l.once.Do(func() { l.err = errors.Join(syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN), l.file.Close()) })
	return l.err
}

func Try(path string) (io.Closer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return &lock{file: f}, nil
}
