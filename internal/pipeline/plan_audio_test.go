package pipeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/platform/queue"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/migrations"
)

func TestAudioPlanForSilentAndAudibleMedia(t *testing.T) {
	dsn := os.Getenv("VH_TEST_AUDIO_PLAN_DB_URL")
	if dsn == "" {
		t.Skip("set VH_TEST_AUDIO_PLAN_DB_URL to videohelper_audio_plan_test; no Redis needed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var name string
	if err := database.QueryRowContext(ctx, "SELECT current_database()").Scan(&name); err != nil || name != "videohelper_audio_plan_test" {
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
	pool, err := db.Open(ctx, dsn, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	const clap = "clap-htsat-unfused@r1"
	span := 10000
	metadata := mlclient.SpacesResponse{Spaces: []mlclient.SpaceInfo{}, Transcribers: []mlclient.TranscriberInfo{{ID: "asr@r1", Languages: []string{"zh"}, MaxWindowMs: 900000, Fingerprint: "sha256:" + strings.Repeat("b", 64)}}}
	for _, id := range []string{"wemm-2b@r1", clap, "clap-shadow@r1"} {
		metadata.Spaces = append(metadata.Spaces, mlclient.SpaceInfo{ID: id, Dim: 512, Metric: "cosine", QueryInputs: []string{"text"}, DocInputs: []string{"video", "text", "audio"}, MaxSpanMs: &span, MaxBatch: 8, Fingerprint: "sha256:" + strings.Repeat("a", 64)})
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(metadata) }))
	defer server.Close()
	registry := space.New(pool, mlclient.New(config.MLConfig{Endpoints: []string{server.URL}, Token: "fixture"}, nil), config.MLConfig{Transcriber: "asr@r1"})
	if err := registry.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	channels := []space.Channel{{Name: "visual", Doc: "video", Seg: "grid@v1", Active: []string{"wemm-2b@r1"}}, {Name: "speech", Doc: "text", Seg: "sentence@v1", Active: []string{"wemm-2b@r1"}}, {Name: "audio", Doc: "audio", Seg: "grid@v1", Active: []string{clap}, Shadow: []string{"clap-shadow@r1"}}}
	for _, media := range []struct {
		name         string
		video, audio bool
	}{
		{"silent video", true, false}, {"video with audio", true, true}, {"audio only", false, true},
	} {
		t.Run(media.name, func(t *testing.T) {
			plan, err := ExpandPlan(library.Video{HasVideo: &media.video, HasAudio: &media.audio}, channels, registry)
			if err != nil {
				t.Fatal(err)
			}
			byName := map[string]StageDef{}
			for _, def := range plan {
				byName[def.Name] = def
			}
			audio, exists := byName["embed:audio:"+clap]
			_, hasASR := byName["asr"]
			_, hasSpeech := byName["embed:speech:wemm-2b@r1"]
			_, hasVisual := byName["embed:visual:wemm-2b@r1"]
			if exists != media.audio || hasASR != media.audio || hasSpeech != media.audio || hasVisual != media.video {
				t.Fatalf("wrong track eligibility: %+v", byName)
			}
			if !media.audio {
				return
			}
			if !audio.Required || audio.Queue != queue.QueueGPU || !slices.Equal(audio.Deps, []string{"prepare_media"}) || audio.Options.Channel != "audio" || audio.Options.Seg != "grid@v1" || audio.Options.Space.Dim != 512 || audio.Options.Space.MaxSpanMs != 10000 {
				t.Fatalf("audio stage plan=%+v", audio)
			}
			shadow := byName["embed:audio:clap-shadow@r1"]
			if shadow.Required || shadow.Queue != queue.QueueGPULow || !slices.Contains(byName["finalize"].Deps, audio.Name) || slices.Contains(byName["finalize"].Deps, shadow.Name) {
				t.Fatalf("audio readiness/shadow contract: %+v", byName)
			}
		})
	}
}
