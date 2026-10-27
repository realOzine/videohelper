// Package store persists a single user's desktop library and managed media.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	sqlitevec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
	"github.com/ozine/videohelper/internal/desktop/filelock"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("state conflict")
	ErrStale    = errors.New("job attempt is no longer current")
	ErrInvalid  = errors.New("invalid input")
	vecOnce     sync.Once
)

//go:embed schema.sql
var schema string

type Store struct {
	db   *sql.DB
	root *os.Root
	dir  string
	lock io.Closer
}

// Tx is valid only inside Checkpoint or CompleteJob. Its output is committed
// atomically with progress and guarded by the job's current attempt.
type Tx struct{ tx *sql.Tx }

func Open(ctx context.Context, dataDir string) (*Store, error) {
	if dataDir == "" {
		return nil, ErrInvalid
	}
	dir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "media"), 0700); err != nil {
		return nil, err
	}
	lock, err := filelock.Try(filepath.Join(dir, "library.lock"))
	if errors.Is(err, filelock.ErrLocked) {
		return nil, ErrConflict
	}
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(filepath.Join(dir, "media"))
	if err != nil {
		lock.Close()
		return nil, err
	}
	vecOnce.Do(sqlitevec.Auto)
	u := url.URL{Scheme: "file", Path: filepath.Join(dir, "library.sqlite")}
	db, err := sql.Open("sqlite3", u.String()+"?_foreign_keys=on&_journal_mode=WAL&_synchronous=FULL&_busy_timeout=5000&_txlock=immediate")
	if err != nil {
		root.Close()
		lock.Close()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, root: root, dir: dir, lock: lock}
	if err := s.init(ctx); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) init(ctx context.Context) error {
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > 1 {
		return fmt.Errorf("database schema %d is newer than this application", version)
	}
	var vecVersion string
	if err := s.db.QueryRowContext(ctx, "SELECT vec_version()").Scan(&vecVersion); err != nil {
		return fmt.Errorf("sqlite-vec initialization: %w", err)
	}
	return s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, schema); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "PRAGMA user_version=1")
		return err
	})
}

func (s *Store) Close() error { return errors.Join(s.db.Close(), s.root.Close(), s.lock.Close()) }

func (s *Store) write(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func timestamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func missing(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
func changed(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
