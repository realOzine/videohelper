// Package filelock provides a nonblocking exclusive OS lock released on process exit.
package filelock

import "errors"

var ErrLocked = errors.New("file is locked by another owner")
