package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/migrations"
)

func TestSpaceAddAudioIntegration(t *testing.T) {
	dsn := os.Getenv("VH_TEST_AUDIO_CLI_DB_URL")
	if dsn == "" {
		t.Skip("set VH_TEST_AUDIO_CLI_DB_URL to videohelper_audio_cli_test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var name string
	if err := database.QueryRowContext(ctx, "SELECT current_database()").Scan(&name); err != nil || name != "videohelper_audio_cli_test" {
		t.Fatalf("refusing database %q: %v", name, err)
	}
	if err := migrations.Up(ctx, database); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := migrations.DownTo(context.Background(), database, 0); err != nil {
			t.Error(err)
		}
	}()
	const id = "clap-htsat-unfused@r1"
	span := 10000
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(mlclient.SpacesResponse{Spaces: []mlclient.SpaceInfo{{ID: id, Dim: 512, Metric: "cosine", QueryInputs: []string{"text"}, DocInputs: []string{"audio"}, MaxSpanMs: &span, MaxBatch: 8, Fingerprint: "sha256:" + strings.Repeat("a", 64)}}, Transcribers: []mlclient.TranscriberInfo{}})
	}))
	defer server.Close()
	t.Setenv("VH_AUDIO_CLI_DB", dsn)
	t.Setenv("VH_AUDIO_CLI_TOKEN", "contract-test")
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := fmt.Sprintf("database:\n  url_env: VH_AUDIO_CLI_DB\n  max_conns: 2\nml:\n  endpoints: [%q]\n  token_env: VH_AUDIO_CLI_TOKEN\n  query_timeout: 1s\n  embed_timeout: 1s\n", server.URL)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(ctx, []string{"-config", path, "space", "add", "audio", id}); err != nil {
		t.Fatal(err)
	}
	pool, err := db.Open(ctx, dsn, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if ready, err := index.VectorIndexReady(ctx, pool, "audio", id); err != nil || !ready {
		t.Fatalf("CLI did not build audio HNSW index: ready=%v error=%v", ready, err)
	}
}
