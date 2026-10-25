package space

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/migrations"
)

func fixtureMetadata() mlclient.SpacesResponse {
	span := 30000
	return mlclient.SpacesResponse{Spaces: []mlclient.SpaceInfo{{ID: "wemm-2b@r1", Dim: 1024, Metric: "cosine", QueryInputs: []string{"text"}, DocInputs: []string{"text", "video", "image"}, MaxSpanMs: &span, MaxBatch: 32, Fingerprint: "sha256:" + strings.Repeat("a", 64)}}, Transcribers: []mlclient.TranscriberInfo{{ID: "paraformer-zh@r1", Languages: []string{"zh"}, MaxWindowMs: 900000, Fingerprint: "sha256:" + strings.Repeat("b", 64)}}}
}

func TestCollectEndpointIdentity(t *testing.T) {
	first := fixtureMetadata()
	second := fixtureMetadata()
	spaces, _, conflicts := collect(map[string]mlclient.SpacesResponse{"http://b": second, "http://a": first})
	if spaces["wemm-2b@r1"].Endpoint != "http://a" || len(conflicts) != 0 {
		t.Fatalf("nondeterministic route or false conflict: %v %v", spaces, conflicts)
	}
	second.Spaces[0].Dim = 2048
	second.Transcribers[0].Fingerprint = "sha256:" + strings.Repeat("c", 64)
	_, transcribers, conflicts := collect(map[string]mlclient.SpacesResponse{"http://b": second, "http://a": first})
	if !conflicts["wemm-2b@r1"] || len(transcribers) != 0 {
		t.Fatal("conflicting endpoint identities were accepted")
	}
}

func TestRegistryIntegration(t *testing.T) {
	dsn := os.Getenv("VH_TEST_SPACE_DB_URL")
	if dsn == "" {
		t.Skip("set VH_TEST_SPACE_DB_URL to an empty disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sqldb.Close()
	var count int
	if err := sqldb.QueryRowContext(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename <> 'goose_db_version'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("refusing nonempty database: count=%d err=%v", count, err)
	}
	if err := migrations.Up(ctx, sqldb); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := migrations.DownTo(ctx, sqldb, 0); err != nil {
			t.Error(err)
		}
	}()
	pool, err := db.Open(ctx, dsn, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var state atomic.Value
	state.Store(fixtureMetadata())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(state.Load()) }))
	defer server.Close()
	var registry *Registry
	client := mlclient.New(config.MLConfig{Endpoints: []string{server.URL}, Token: "test"}, func(id string) (string, bool) { return registry.Resolve(id) })
	registry = New(pool, client)
	if err := registry.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if endpoint, ok := registry.Resolve("wemm-2b@r1"); !ok || endpoint != server.URL {
		t.Fatalf("route=%q %v", endpoint, ok)
	}
	info, ok := registry.Get("wemm-2b@r1")
	if !ok || info.MaxSpanMs != 30000 {
		t.Fatalf("metadata=%+v %v", info, ok)
	}
	info.DocInputs[0] = "mutated"
	info, _ = registry.Get(info.ID)
	if info.DocInputs[0] == "mutated" {
		t.Fatal("caller mutated registry")
	}
	model, _, ok := registry.Transcriber()
	if !ok || model.ID != "paraformer-zh@r1" {
		t.Fatal("missing ASR")
	}
	channels, err := NewChannels(config.ChannelsConfig{"visual": {Doc: "video", Seg: "grid@v1", Weight: 1, Active: []string{info.ID}}, "speech": {Doc: "text", Seg: "sentence@v1", Weight: 1, Active: []string{info.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := channels.Validate(ctx, registry); err == nil {
		t.Fatal("missing HNSW indexes accepted")
	}
	for _, channel := range channels.List() {
		if err := index.EnsureVectorIndex(ctx, pool, channel.Name, index.VectorSpace{ID: info.ID, Dim: info.Dim, Metric: info.Metric}); err != nil {
			t.Fatal(err)
		}
	}
	if err := channels.Validate(ctx, registry); err != nil {
		t.Fatal(err)
	}
	changed := fixtureMetadata()
	changed.Spaces[0].Fingerprint = "sha256:" + strings.Repeat("d", 64)
	state.Store(changed)
	if err := registry.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Resolve("wemm-2b@r1"); ok {
		t.Fatal("drifting fingerprint remained routable")
	}
	var original string
	var disabled bool
	if err := pool.QueryRow(ctx, "SELECT fingerprint,disabled FROM embedding_space WHERE id='wemm-2b@r1'").Scan(&original, &disabled); err != nil || !disabled || original != fixtureMetadata().Spaces[0].Fingerprint {
		t.Fatalf("identity overwritten: %q %v %v", original, disabled, err)
	}
	state.Store(fixtureMetadata())
	if err := registry.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.Resolve("wemm-2b@r1"); ok {
		t.Fatal("disabled space silently reactivated")
	}
	server.Close()
	if err := registry.Sync(ctx); err == nil {
		t.Fatal("all endpoints unavailable should fail")
	}
	if _, ok := registry.Resolve("wemm-2b@r1"); ok {
		t.Fatal("unavailable route retained")
	}
	if _, _, ok := registry.Transcriber(); ok {
		t.Fatal("unavailable transcriber retained")
	}
}
