package index

import (
	"context"
	"strings"
	"testing"

	"github.com/ozine/videohelper/internal/timeline"
)

func TestSpaceCoverageEmptyScope(t *testing.T) {
	counts, err := SpaceCoverage(context.Background(), nil, 1, []int64{}, "visual", "grid@v1", "model@r1", "test")
	if err != nil || counts != (CoverageCounts{}) {
		t.Fatalf("empty scope = %+v, %v", counts, err)
	}
}

func TestSpaceCoveragePostgresIntegration(t *testing.T) {
	ctx, pool := indexTestDB(t)
	workspace := insertWorkspace(t, ctx, pool, "coverage")
	otherWorkspace := insertWorkspace(t, ctx, pool, "other")
	const output = `{"plan":{"options":{"channel":"visual","seg":"grid@v1","space":{"ID":"model@r1","Fingerprint":"test"}}}}`
	addStage := func(videoID int64, channel, status, data string) {
		t.Helper()
		if _, err := pool.Exec(ctx, `INSERT INTO stage_run(video_id,stage,version,status,output)
VALUES($1,$2,'v1',$3,$4)`, videoID, "embed:"+channel+":model@r1", status, data); err != nil {
			t.Fatal(err)
		}
	}
	var completed, partial, foreign int64
	for _, tc := range []struct {
		name, status, stageStatus, output string
		foreign, deleted, indexed         bool
	}{
		{name: "completed", stageStatus: "done", indexed: true},
		{name: "partial", stageStatus: "running"},
		{name: "queued", stageStatus: "queued"},
		{name: "failed", stageStatus: "failed"},
		{name: "no stage"},
		{name: "wrong channel", stageStatus: "done", output: strings.Replace(output, `"channel":"visual"`, `"channel":"audio"`, 1)},
		{name: "wrong segmentation", stageStatus: "done", output: strings.Replace(output, "grid@v1", "grid@v2", 1)},
		{name: "wrong space", stageStatus: "done", output: strings.Replace(output, "model@r1", "other@r1", 1)},
		{name: "wrong fingerprint", stageStatus: "done", output: strings.Replace(output, `"Fingerprint":"test"`, `"Fingerprint":"old"`, 1)},
		{name: "missing frozen plan", stageStatus: "done", output: `{}`},
		{name: "foreign workspace", foreign: true, stageStatus: "done"},
		{name: "deleted", deleted: true, stageStatus: "done"},
		{name: "not ready", status: "indexing", stageStatus: "done"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws, status, data := workspace, tc.status, tc.output
			if tc.foreign {
				ws = otherWorkspace
			}
			if status == "" {
				status = "ready"
			}
			if data == "" {
				data = output
			}
			video := insertVideo(t, ctx, pool, ws, status, tc.deleted)
			if _, err := pool.Exec(ctx, "UPDATE video SET has_video=true,has_audio=true WHERE id=$1", video); err != nil {
				t.Fatal(err)
			}
			if tc.stageStatus != "" {
				addStage(video, "visual", tc.stageStatus, data)
			}
			want := CoverageCounts{EligibleVideos: 1}
			if tc.foreign || tc.deleted || status != "ready" {
				want.EligibleVideos = 0
			}
			if tc.indexed {
				want.IndexedVideos = 1
				completed = video
			}
			if tc.name == "partial" {
				partial = video
			}
			if tc.foreign {
				foreign = video
			}
			got, err := SpaceCoverage(ctx, pool, workspace, []int64{video}, "visual", "grid@v1", "model@r1", "test")
			if err != nil || got != want {
				t.Fatalf("coverage = %+v, %v; want %+v", got, err, want)
			}
		})
	}
	insertSpace(t, ctx, pool, VectorSpace{ID: "model@r1", Dim: 3, Metric: "cosine"})
	if err := UpsertVectors(ctx, pool, []VectorRow{{VideoID: partial, WorkspaceID: workspace, Channel: "visual", SpaceID: "model@r1", Seg: "grid@v1", Span: timeline.Span{StartMs: 0, EndMs: 10}, Vec: []float32{1, 0, 0}}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		ids  []int64
		want CoverageCounts
	}{
		{"workspace", nil, CoverageCounts{EligibleVideos: 10, IndexedVideos: 1}},
		{"selected videos", []int64{completed, completed, partial, foreign}, CoverageCounts{EligibleVideos: 2, IndexedVideos: 1}},
		{"empty", []int64{}, CoverageCounts{}},
		{"missing video", []int64{-1}, CoverageCounts{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SpaceCoverage(ctx, pool, workspace, tc.ids, "visual", "grid@v1", "model@r1", "test")
			if err != nil || got != tc.want {
				t.Fatalf("coverage = %+v, %v; want %+v", got, err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		name               string
		hasVideo, hasAudio *bool
	}{
		{"both", boolPointer(true), boolPointer(true)},
		{"video only", boolPointer(true), boolPointer(false)},
		{"audio only", boolPointer(false), boolPointer(true)},
		{"neither", boolPointer(false), boolPointer(false)},
		{"unknown", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			video := insertVideo(t, ctx, pool, workspace, "ready", false)
			if _, err := pool.Exec(ctx, "UPDATE video SET has_video=$2,has_audio=$3 WHERE id=$1", video, tc.hasVideo, tc.hasAudio); err != nil {
				t.Fatal(err)
			}
			for _, channel := range []string{"visual", "speech", "audio"} {
				addStage(video, channel, "done", strings.Replace(output, `"channel":"visual"`, `"channel":"`+channel+`"`, 1))
				want := CoverageCounts{}
				media := tc.hasAudio
				if channel == "visual" {
					media = tc.hasVideo
				}
				if media != nil && *media {
					want = CoverageCounts{EligibleVideos: 1, IndexedVideos: 1}
				}
				got, err := SpaceCoverage(ctx, pool, workspace, []int64{video}, channel, "grid@v1", "model@r1", "test")
				if err != nil || got != want {
					t.Fatalf("%s coverage without vectors = %+v, %v; want %+v", channel, got, err, want)
				}
			}
		})
	}
}

func boolPointer(value bool) *bool { return &value }
