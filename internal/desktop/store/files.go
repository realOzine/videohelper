package store

import (
	"context"
	"crypto/rand"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var keySegment = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*$`)
var sourceExt = regexp.MustCompile(`^\.[a-z0-9]+$`)

func validKey(key string) error {
	parts := strings.Split(key, "/")
	if !fs.ValidPath(key) || strings.ContainsAny(key, "\\:\x00") || len(parts) < 3 || parts[0] != "videos" {
		return ErrInvalid
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || id <= 0 || strconv.FormatInt(id, 10) != parts[1] {
		return ErrInvalid
	}
	for _, part := range parts {
		if !keySegment.MatchString(part) {
			return ErrInvalid
		}
	}
	return nil
}

func VideoKey(id int64, name string) string {
	return "videos/" + strconv.FormatInt(id, 10) + "/" + name
}

func (s *Store) OpenFile(key string) (*os.File, error) {
	if err := validKey(key); err != nil {
		return nil, err
	}
	parts := strings.Split(key, "/")
	for i := range parts {
		info, err := s.root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrInvalid
		}
	}
	f, err := s.root.Open(key)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		if err == nil {
			err = ErrInvalid
		}
		return nil, err
	}
	return f, nil
}

func (s *Store) MediaPath(key string) (string, error) {
	f, err := s.OpenFile(key)
	if err != nil {
		return "", err
	}
	f.Close()
	return filepath.Join(s.dir, "media", filepath.FromSlash(key)), nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func (s *Store) WriteFile(ctx context.Context, key string, r io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r == nil {
		return ErrInvalid
	}
	if err := validKey(key); err != nil {
		return err
	}
	if err := s.root.MkdirAll(path.Dir(key), 0700); err != nil {
		return err
	}
	tmp := key + "." + rand.Text() + ".part"
	f, err := s.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { f.Close(); s.root.Remove(tmp) }()
	if _, err := io.Copy(f, contextReader{ctx, r}); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return s.root.Rename(tmp, key)
}

func (s *Store) CopyFile(ctx context.Context, videoID int64, sourcePath string) (string, error) {
	if videoID <= 0 {
		return "", ErrInvalid
	}
	f, err := os.Open(sourcePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return "", ErrInvalid
	}
	ext := strings.ToLower(filepath.Ext(sourcePath))
	if len(ext) < 2 || len(ext) > 12 || !sourceExt.MatchString(ext) {
		ext = ".bin"
	}
	key := VideoKey(videoID, "source"+ext)
	if err := s.WriteFile(ctx, key, f); err != nil {
		return "", err
	}
	return key, nil
}
