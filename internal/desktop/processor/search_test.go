package processor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ozine/videohelper/internal/desktop/store"
)

func TestSearchValidatesOverridesBeforeEmptyScopeAndChannelFilters(t *testing.T) {
	ctx := context.Background()
	s, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	f, server := newFakeML(t)
	p, err := New(ctx, s, Config{Endpoints: []string{server.URL}, Token: "test", VisualSpace: "visual@r1", AudioSpace: "audio@r1", Transcriber: "asr@r1"})
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, q Query, want error) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			q.Text = "原样查询"
			result, err := p.Search(ctx, q)
			if !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
			if len(result.Moments) != 0 {
				t.Fatalf("unexpected recall before override validation: %+v", result)
			}
		})
	}
	check("empty library invalid ID", Query{SpaceOverride: map[string]string{"visual": "missing"}}, store.ErrConflict)
	check("empty library valid ID", Query{SpaceOverride: map[string]string{"visual": p.identity["visual"]}}, nil)
	check("empty IDs invalid disabled channel", Query{VideoIDs: []int64{}, SpaceOverride: map[string]string{"audio": "missing"}}, store.ErrConflict)

	source := filepath.Join(t.TempDir(), "fixture.mp4")
	if err := os.WriteFile(source, []byte("coverage fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	v, err := p.ImportFile(ctx, source, nil)
	if err != nil {
		t.Fatal(err)
	}
	j, err := s.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJob(ctx, j.ID, j.Attempt, func(tx *store.Tx) error {
		return tx.UpdateMedia(ctx, v.ID, store.MediaInfo{DurationMs: 6000, HasVideo: true, HasAudio: true})
	}); err != nil {
		t.Fatal(err)
	}
	check("disabled channel invalid ID", Query{ChannelWeights: map[string]float64{"audio": 0}, SpaceOverride: map[string]string{"audio": "missing"}}, store.ErrConflict)
	check("disabled channel incomplete coverage", Query{ChannelWeights: map[string]float64{"audio": 0}, SpaceOverride: map[string]string{"audio": p.identity["audio"]}}, store.ErrConflict)
	check("excluded channel incomplete coverage", Query{OnlyChannels: []string{"speech"}, SpaceOverride: map[string]string{"visual": p.identity["visual"]}}, store.ErrConflict)
	emptyProject, err := s.CreateProject(ctx, "empty scope")
	if err != nil {
		t.Fatal(err)
	}
	check("empty project valid zero coverage", Query{ProjectID: &emptyProject.ID, SpaceOverride: map[string]string{"visual": p.identity["visual"]}}, nil)
	check("empty project invalid ID", Query{ProjectID: &emptyProject.ID, SpaceOverride: map[string]string{"audio": "missing"}}, store.ErrConflict)
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.inputs) != 0 {
		t.Fatalf("invalid or empty scoped overrides invoked model encoding: %+v", f.inputs)
	}
}
