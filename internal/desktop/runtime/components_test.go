package runtime

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestComponentVerifiedAndCorruptionRecovered(t *testing.T) {
	data := []byte("verified tool")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.Write(data) }))
	defer server.Close()
	digest := sha256.Sum256(data)
	c := component{Name: "tool", URL: server.URL, SHA256: hex.EncodeToString(digest[:]), Format: "raw"}
	root := t.TempDir()
	path, err := installComponent(context.Background(), server.Client(), root, c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := installComponent(context.Background(), server.Client(), root, c); err != nil || requests != 1 {
		t.Fatalf("cache: %d %v", requests, err)
	}
	if err := os.WriteFile(path, []byte("broken"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := installComponent(context.Background(), server.Client(), root, c); err != nil || requests != 2 {
		t.Fatalf("recover: %d %v", requests, err)
	}
	c.SHA256 = string(bytes.Repeat([]byte("0"), 64))
	if _, err := installComponent(context.Background(), server.Client(), root, c); err == nil {
		t.Fatal("accepted wrong checksum")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatalf("failed install left files: %v", entries)
	}
}

func TestExtractionNeverUsesArchivePaths(t *testing.T) {
	for _, format := range []string{"tar.gz", "zip", "gzip"} {
		t.Run(format, func(t *testing.T) {
			var data bytes.Buffer
			switch format {
			case "tar.gz":
				gz := gzip.NewWriter(&data)
				tw := tar.NewWriter(gz)
				content := []byte("tool")
				tw.WriteHeader(&tar.Header{Name: "../../tool", Mode: 0700, Size: int64(len(content))})
				tw.Write(content)
				tw.Close()
				gz.Close()
			case "zip":
				zw := zip.NewWriter(&data)
				w, _ := zw.Create("../../tool")
				w.Write([]byte("tool"))
				zw.Close()
			case "gzip":
				gz := gzip.NewWriter(&data)
				gz.Write([]byte("tool"))
				gz.Close()
			}
			dir := t.TempDir()
			archive := filepath.Join(dir, "artifact")
			os.WriteFile(archive, data.Bytes(), 0600)
			output := filepath.Join(dir, "safe-output")
			if err := extractExecutable(archive, output, format, "tool"); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(output)
			if err != nil || string(got) != "tool" {
				t.Fatalf("%q %v", got, err)
			}
		})
	}
}

func TestPinnedComponentCatalog(t *testing.T) {
	for _, platform := range []string{"darwin/arm64", "darwin/amd64", "linux/amd64", "windows/amd64"} {
		components, err := componentsFor(platform)
		if err != nil {
			t.Fatal(err)
		}
		if len(components) != 7 {
			t.Fatalf("%s: incomplete tools/notices", platform)
		}
		for _, component := range components {
			if len(component.SHA256) != 64 || bytes.Contains([]byte(component.URL), []byte("/latest/")) {
				t.Fatalf("unpinned component: %+v", component)
			}
		}
	}
}
