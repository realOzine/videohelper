package runtime

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

//go:embed components.json
var componentManifest []byte

//go:embed python-downloads.json
var pythonDownloads []byte

type component struct {
	Name, URL, SHA256, Format string
	Notice                    bool
}

type Tools struct {
	FFmpeg  string `json:"ffmpeg"`
	FFprobe string `json:"ffprobe"`
	YTDLP   string `json:"yt_dlp"`
	Deno    string `json:"deno"`
}

func componentsFor(platform string) ([]component, error) {
	var platforms map[string][]component
	if err := json.Unmarshal(componentManifest, &platforms); err != nil {
		return nil, err
	}
	components, ok := platforms[platform]
	if !ok {
		return nil, fmt.Errorf("desktop runtime does not support %s", platform)
	}
	return components, nil
}

func executable(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func fileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Only a verified complete artifact becomes executable; failed downloads stay private and temporary.
func installComponent(ctx context.Context, client *http.Client, root string, c component) (string, error) {
	if len(c.SHA256) != 64 {
		return "", errors.New("component has no pinned SHA256")
	}
	if _, err := hex.DecodeString(c.SHA256); err != nil {
		return "", err
	}
	dir := filepath.Join(root, c.Name+"-"+c.SHA256[:16])
	name := executable(c.Name)
	if c.Notice {
		name = c.Name
	}
	dst := filepath.Join(dir, name)
	if expected, err := os.ReadFile(dst + ".sha256"); err == nil {
		if actual, err := fileHash(dst); err == nil && actual == string(expected) {
			return dst, nil
		}
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(root, ".download-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	archive := filepath.Join(stage, "artifact")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "VideoHelper/0.1")
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", c.Name, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", c.Name, response.StatusCode)
	}
	f, err := os.OpenFile(archive, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, h), io.LimitReader(response.Body, 256<<20+1))
	closeErr := f.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if n > 256<<20 {
		return "", errors.New("component download exceeds 256 MiB")
	}
	if hex.EncodeToString(h.Sum(nil)) != c.SHA256 {
		return "", fmt.Errorf("%s checksum mismatch", c.Name)
	}
	output := filepath.Join(stage, name)
	if err := extractExecutable(archive, output, c.Format, name); err != nil {
		return "", fmt.Errorf("extract %s: %w", c.Name, err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	digest, err := fileHash(output)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(output+".sha256", []byte(digest), 0600); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	// The caller serializes installs; preserve unrelated components and previous versions.
	if err := os.Rename(output, dst); err != nil {
		return "", err
	}
	if err := os.Rename(output+".sha256", dst+".sha256"); err != nil {
		return "", err
	}
	return dst, nil
}

func extractExecutable(archive, dst, format, name string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	write := func(reader io.Reader) error {
		out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
		if err != nil {
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(reader, 512<<20+1))
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if n == 0 || n > 512<<20 {
			return errors.New("invalid component size")
		}
		return nil
	}
	switch format {
	case "raw":
		return write(f)
	case "gzip", "tar.gz":
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		defer gz.Close()
		if format == "gzip" {
			return write(gz)
		}
		reader := tar.NewReader(gz)
		for {
			header, err := reader.Next()
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
			if filepath.Base(header.Name) == name && header.Typeflag == tar.TypeReg {
				return write(reader)
			}
		}
	case "zip":
		info, err := f.Stat()
		if err != nil {
			return err
		}
		reader, err := zip.NewReader(f, info.Size())
		if err != nil {
			return err
		}
		for _, entry := range reader.File {
			if filepath.Base(entry.Name) != name || !entry.Mode().IsRegular() {
				continue
			}
			r, err := entry.Open()
			if err != nil {
				return err
			}
			err = write(r)
			r.Close()
			return err
		}
	default:
		return fmt.Errorf("unsupported archive format %s", format)
	}
	return fmt.Errorf("archive does not contain %s", name)
}

func downloadClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || req.URL.Scheme != "https" {
			return errors.New("unsafe component redirect")
		}
		return nil
	}}
}

func toolsPath(tools Tools) string {
	return strings.Join([]string{filepath.Dir(tools.FFmpeg), filepath.Dir(tools.FFprobe), filepath.Dir(tools.YTDLP), filepath.Dir(tools.Deno)}, string(os.PathListSeparator))
}
