//go:build windows

package filelock

import (
	"errors"
	"io"
	"os"
	"sync"

	"golang.org/x/sys/windows"
)

type lock struct {
	file    *os.File
	overlap windows.Overlapped
	once    sync.Once
	err     error
}

func (l *lock) Close() error {
	l.once.Do(func() {
		l.err = errors.Join(windows.UnlockFileEx(windows.Handle(l.file.Fd()), 0, 1, 0, &l.overlap), l.file.Close())
	})
	return l.err
}

func Try(path string) (io.Closer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	l := &lock{file: f}
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &l.overlap); err != nil {
		f.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, ErrLocked
		}
		return nil, err
	}
	return l, nil
}
