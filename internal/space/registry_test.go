package space

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
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

func TestRegistryListSnapshot(t *testing.T) {
	r := New(nil, nil, config.MLConfig{})
	if list := r.List(); list == nil || len(list) != 0 {
		t.Fatalf("empty registry list=%v", list)
	}
	r.spaces = map[string]Info{
		"z@r1": {ID: "z@r1", Disabled: true, Endpoint: "http://ml", DocInputs: []string{"video"}, QueryInputs: []string{"text"}},
		"a@r1": {ID: "a@r1", DocInputs: []string{"audio"}, QueryInputs: []string{"text"}},
	}
	list := r.List()
	if len(list) != 2 || list[0].ID != "a@r1" || list[1].ID != "z@r1" || !list[1].Disabled || list[0].Endpoint != "" {
		t.Fatalf("snapshot dropped or reordered unroutable models: %+v", list)
	}
	list[0].DocInputs[0], list[0].QueryInputs[0], list[1].ID = "mutated", "mutated", "mutated"
	again := r.List()
	if !slices.Equal(again[0].DocInputs, []string{"audio"}) || !slices.Equal(again[0].QueryInputs, []string{"text"}) || again[1].ID != "z@r1" {
		t.Fatalf("caller mutated registry snapshot: %+v", again)
	}
}

func fixtureMetadata() mlclient.SpacesResponse {
	span := 30000
	return mlclient.SpacesResponse{Spaces: []mlclient.SpaceInfo{{ID: "wemm-2b@r1", Dim: 1024, Metric: "cosine", QueryInputs: []string{"text"}, DocInputs: []string{"text", "video", "image"}, MaxSpanMs: &span, MaxBatch: 32, Fingerprint: "sha256:" + strings.Repeat("a", 64)}}, Transcribers: []mlclient.TranscriberInfo{{ID: "paraformer-zh@r1", Languages: []string{"zh"}, MaxWindowMs: 900000, Fingerprint: "sha256:" + strings.Repeat("b", 64)}}}
}

func TestCollectEndpointBindings(t *testing.T) {
	for _, selected := range []string{"", "http://a", "http://b", "http://missing"} {
		t.Run(selected, func(t *testing.T) {
			first, second := fixtureMetadata(), fixtureMetadata()
			bindings := map[string]string{"wemm-2b@r1": selected, "paraformer-zh@r1": selected}
			endpoints := map[string]mlclient.SpacesResponse{"http://b": second, "http://a": first}
			spaces, models, conflicts := collect(endpoints, bindings)
			available := selected == "http://a" || selected == "http://b"
			if (spaces["wemm-2b@r1"].Endpoint != "") != available || len(conflicts) != 0 {
				t.Fatalf("wrong embedding provider: %v %v", spaces, conflicts)
			}
			model, ok := models["paraformer-zh@r1"]
			if ok != available || available && (model.endpoint != selected || spaces["wemm-2b@r1"].Endpoint != selected) {
				t.Fatalf("wrong transcriber or space binding: %v %v", spaces, models)
			}
			// A binding cannot conceal a conflicting identity at the other provider.
			second.Spaces[0].Dim = 2048
			second.Transcribers[0].Fingerprint = "sha256:" + strings.Repeat("c", 64)
			_, models, conflicts = collect(endpoints, bindings)
			if !conflicts["wemm-2b@r1"] || len(models) != 0 {
				t.Fatal("conflicting endpoint identities were accepted")
			}
		})
	}
}

func TestCollectUnavailableBoundProvider(t *testing.T) {
	metadata := fixtureMetadata()
	bindings := map[string]string{"wemm-2b@r1": "http://missing", "paraformer-zh@r1": "http://missing"}
	registry := New(nil, nil, config.MLConfig{Transcriber: "paraformer-zh@r1", ModelEndpoints: bindings})
	bindings["wemm-2b@r1"] = "http://healthy"
	registry.spaces, registry.transcribers, _ = collect(map[string]mlclient.SpacesResponse{"http://healthy": metadata}, registry.bindings)
	if _, ok := registry.Get("wemm-2b@r1"); ok {
		t.Fatal("unavailable bound embedding provider fell back or caller mutated bindings")
	}
	if _, _, ok := registry.Transcriber(); ok {
		t.Fatal("unavailable bound ASR provider fell back")
	}
}

func TestConfiguredAndFrozenTranscribers(t *testing.T) {
	metadata := fixtureMetadata()
	old := metadata.Transcribers[0]
	replacement := old
	replacement.ID = "replacement-asr@r1"
	replacement.Fingerprint = "sha256:" + strings.Repeat("c", 64)
	metadata.Transcribers = append(metadata.Transcribers, replacement)
	_, models, _ := collect(map[string]mlclient.SpacesResponse{"http://ml": metadata}, nil)
	registry := New(nil, nil, config.MLConfig{Transcriber: replacement.ID})
	registry.transcribers = models
	selected, endpoint, ok := registry.Transcriber()
	if !ok || selected.ID != replacement.ID || endpoint != "http://ml" {
		t.Fatalf("configured model not selected: %+v %q %v", selected, endpoint, ok)
	}
	frozen, _, ok := registry.GetTranscriber(old.ID)
	if !ok || frozen.ID != old.ID || frozen.Fingerprint != old.Fingerprint {
		t.Fatalf("frozen model unavailable after selection switch: %+v %v", frozen, ok)
	}
	frozen.Languages[0] = "mutated"
	frozen, _, _ = registry.GetTranscriber(old.ID)
	if frozen.Languages[0] != "zh" {
		t.Fatal("caller mutated transcriber metadata")
	}
	delete(registry.transcribers, old.ID)
	if _, _, ok := registry.GetTranscriber(old.ID); ok {
		t.Fatal("removed frozen model fell back to configured replacement")
	}
	for _, id := range []string{"", "missing@r1"} {
		registry.selectedASR = id
		if _, _, ok := registry.Transcriber(); ok {
			t.Fatalf("missing configured model %q fell back to an available model", id)
		}
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
	registry = New(pool, client, config.MLConfig{Transcriber: "paraformer-zh@r1"})
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
	duplicate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(state.Load()) }))
	defer duplicate.Close()
	cfg := config.MLConfig{Transcriber: model.ID, Endpoints: []string{server.URL, duplicate.URL}, Token: "test"}
	ambiguous := New(pool, mlclient.New(cfg, nil), cfg)
	if err := ambiguous.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := ambiguous.Resolve(info.ID); ok {
		t.Fatal("ambiguous embedding provider remained routable")
	}
	if _, _, ok := ambiguous.Transcriber(); ok {
		t.Fatal("ambiguous ASR provider remained routable")
	}
	var disabledByAmbiguity bool
	if err := pool.QueryRow(ctx, "SELECT disabled FROM embedding_space WHERE id=$1", info.ID).Scan(&disabledByAmbiguity); err != nil || disabledByAmbiguity {
		t.Fatalf("provider ambiguity permanently disabled identity: %v %v", disabledByAmbiguity, err)
	}
	cfg.ModelEndpoints = map[string]string{info.ID: duplicate.URL, model.ID: duplicate.URL}
	bound := New(pool, mlclient.New(cfg, nil), cfg)
	if err := bound.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	endpoint, routed := bound.Resolve(info.ID)
	_, asrEndpoint, asrRouted := bound.Transcriber()
	if !routed || !asrRouted || endpoint != duplicate.URL || asrEndpoint != duplicate.URL {
		t.Fatalf("explicit bindings ignored: space=%q/%v asr=%q/%v", endpoint, routed, asrEndpoint, asrRouted)
	}
	duplicate.Close()
	if err := bound.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	_, routed = bound.Resolve(info.ID)
	_, _, asrRouted = bound.Transcriber()
	if routed || asrRouted {
		t.Fatal("bound provider outage fell back to another endpoint")
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
