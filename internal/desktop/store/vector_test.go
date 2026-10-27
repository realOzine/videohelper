package store

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/ozine/videohelper/internal/timeline"
)

func fixtureSpace(t *testing.T, s *Store) Space {
	t.Helper()
	sp, err := s.RegisterSpace(context.Background(), Space{ID: "test@r1", Fingerprint: "cpu-fp", Dim: 2, Metric: "cosine", DocInputs: []string{"video", "text"}, QueryInputs: []string{"text"}})
	if err != nil {
		t.Fatal(err)
	}
	return sp
}

func readyVideo(t *testing.T, s *Store, v Video) {
	t.Helper()
	ctx := context.Background()
	if _, err := s.CreateJob(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	j, err := s.ClaimJob(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteJob(ctx, j.ID, j.Attempt, nil); err != nil {
		t.Fatal(err)
	}
}

func TestSQLiteVecKNNFiltersAndPurge(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	sp := fixtureSpace(t, s)
	a, b, c, d := fixtureVideo(t, s), fixtureVideo(t, s), fixtureVideo(t, s), fixtureVideo(t, s)
	for _, v := range []Video{a, b, c} {
		readyVideo(t, s, v)
	}
	rows := []VectorRow{
		{VideoID: a.ID, SpaceID: sp.ID, Fingerprint: sp.Fingerprint, Channel: "visual", Seg: "grid@v1", Span: timeline.Span{StartMs: 0, EndMs: 6000}, Vec: []float32{1, 0}},
		{VideoID: b.ID, SpaceID: sp.ID, Fingerprint: sp.Fingerprint, Channel: "visual", Seg: "grid@v1", Span: timeline.Span{StartMs: 6000, EndMs: 12000}, Vec: []float32{0.9, 0.1}},
		{VideoID: c.ID, SpaceID: sp.ID, Fingerprint: sp.Fingerprint, Channel: "visual", Seg: "grid@v1", Span: timeline.Span{StartMs: 0, EndMs: 6000}, Vec: []float32{1, 0}},
		{VideoID: d.ID, SpaceID: sp.ID, Fingerprint: sp.Fingerprint, Channel: "visual", Seg: "grid@v1", Span: timeline.Span{StartMs: 0, EndMs: 6000}, Vec: []float32{1, 0}},
	}
	if err := s.PutVectors(ctx, rows); err != nil {
		t.Fatal(err)
	}
	q := VectorQuery{SpaceID: sp.ID, Fingerprint: sp.Fingerprint, Channel: "visual", Seg: "grid@v1", Vec: []float32{1, 0}, K: 10}
	hits, err := s.SearchVectors(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 || hits[0].VideoID != a.ID || hits[1].VideoID != c.ID || hits[2].VideoID != b.ID || hits[2].Rank != 3 {
		t.Fatalf("stable ready KNN: %v", hits)
	}
	p, err := s.CreateProject(ctx, "target")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetProjectVideos(ctx, p.ID, []int64{b.ID}); err != nil {
		t.Fatal(err)
	}
	q.K = 1
	q.ProjectID = &p.ID
	hits, err = s.SearchVectors(ctx, q)
	if err != nil || len(hits) != 1 || hits[0].VideoID != b.ID {
		t.Fatalf("project must filter before topK: %v %v", hits, err)
	}
	q.ProjectID = nil
	q.VideoIDs = []int64{b.ID}
	hits, err = s.SearchVectors(ctx, q)
	if err != nil || len(hits) != 1 || hits[0].VideoID != b.ID {
		t.Fatalf("video filter: %v %v", hits, err)
	}
	q.VideoIDs = nil
	q.TimeRange = &timeline.Span{StartMs: 6000, EndMs: 7000}
	hits, err = s.SearchVectors(ctx, q)
	if err != nil || len(hits) != 1 || hits[0].VideoID != b.ID {
		t.Fatalf("half-open time filter: %v %v", hits, err)
	}
	q.TimeRange = nil
	q.K = 10
	if err := s.DeleteVideo(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	hits, err = s.SearchVectors(ctx, q)
	if err != nil || len(hits) != 2 || hits[0].VideoID != c.ID {
		t.Fatalf("soft deleted leak: %v %v", hits, err)
	}
	if restored, err := s.RestoreVideo(ctx, a.ID); err != nil || restored.Status != "ready" || restored.SourceKey != a.SourceKey {
		t.Fatalf("ready media changed on restore: %+v %v", restored, err)
	}
	hits, err = s.SearchVectors(ctx, q)
	if err != nil || len(hits) != 3 || hits[0].VideoID != a.ID {
		t.Fatalf("ready index was not preserved: %v %v", hits, err)
	}
	if err := s.DeleteVideo(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.PurgeVideo(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM "+vectorTable(sp.ID)).Scan(&n); err != nil || n != 3 {
		t.Fatalf("orphan vec0 rows: %d %v", n, err)
	}
	if _, err := s.RestoreVideo(ctx, c.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	q.VideoIDs = []int64{}
	hits, err = s.SearchVectors(ctx, q)
	if err != nil || len(hits) != 0 {
		t.Fatalf("empty scope: %v %v", hits, err)
	}
}

func TestVectorsIdentityAndRollback(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	sp := fixtureSpace(t, s)
	v := fixtureVideo(t, s)
	row := VectorRow{VideoID: v.ID, SpaceID: sp.ID, Fingerprint: sp.Fingerprint, Channel: "speech", Seg: "sentence@v1", Span: timeline.Span{StartMs: 0, EndMs: 6000}, Vec: []float32{1, 0}}
	bad := row
	bad.Span.StartMs = 6000
	bad.Span.EndMs = 12000
	bad.Vec = []float32{float32(math.NaN()), 0}
	if err := s.PutVectors(ctx, []VectorRow{row, bad}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if n, err := s.VectorCount(ctx, v.ID, row.Channel, sp.ID); err != nil || n != 0 {
		t.Fatalf("partial vector batch committed: %d %v", n, err)
	}
	if err := s.PutVectors(ctx, []VectorRow{row}); err != nil {
		t.Fatal(err)
	}
	row.Vec = []float32{0, 1}
	if err := s.PutVectors(ctx, []VectorRow{row}); err != nil {
		t.Fatal(err)
	}
	if n, err := s.VectorCount(ctx, v.ID, row.Channel, sp.ID); err != nil || n != 1 {
		t.Fatalf("upsert duplicated: %d %v", n, err)
	}
	changed := sp
	changed.Fingerprint = "mps-fp"
	disabled, err := s.RegisterSpace(ctx, changed)
	if !errors.Is(err, ErrConflict) || !disabled.Disabled || disabled.Fingerprint != sp.Fingerprint {
		t.Fatalf("identity overwrite: %+v %v", disabled, err)
	}
	if err := s.PutVectors(ctx, []VectorRow{row}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if got, err := s.Space(ctx, sp.ID); err != nil || !got.Disabled || !reflect.DeepEqual(got.DocInputs, sp.DocInputs) {
		t.Fatalf("%+v %v", got, err)
	}
	if n, err := s.VectorCount(ctx, v.ID, row.Channel, sp.ID); err != nil || n != 1 {
		t.Fatalf("old index destroyed: %d %v", n, err)
	}
}

func TestSentencesRollbackAndRange(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	v := fixtureVideo(t, s)
	first := Sentence{StartMs: 0, EndMs: 6000, Text: "first", Model: "asr@r1", Words: []Word{{W: "first", S: 0, E: 1000}}}
	if err := s.ReplaceSentences(ctx, v.ID, []Sentence{first}); err != nil {
		t.Fatal(err)
	}
	bad := first
	bad.EndMs = -1
	if err := s.ReplaceSentences(ctx, v.ID, []Sentence{bad}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	got, err := s.Sentences(ctx, v.ID, nil)
	if err != nil || len(got) != 1 || got[0].Text != "first" {
		t.Fatalf("%v %v", got, err)
	}
	got, err = s.Sentences(ctx, v.ID, &timeline.Span{StartMs: 6000, EndMs: 7000})
	if err != nil || len(got) != 0 {
		t.Fatalf("half-open range: %v %v", got, err)
	}
}
