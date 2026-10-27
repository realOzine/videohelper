package filelock

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestExclusiveUntilClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owner.lock")
	owner, err := Try(path)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if other, err := Try(path); !errors.Is(err, ErrLocked) {
		if other != nil {
			other.Close()
		}
		t.Fatalf("second owner: %v", err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	other, err := Try(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
}
