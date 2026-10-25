package index

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/timeline"
)

func TestSpeechPostgresIntegration(t *testing.T) {
	ctx, pool := indexTestDB(t)
	w1 := insertWorkspace(t, ctx, pool, "first")
	w2 := insertWorkspace(t, ctx, pool, "second")
	v1 := insertVideo(t, ctx, pool, w1, "ready", false)
	v2 := insertVideo(t, ctx, pool, w1, "indexing", false)
	space := VectorSpace{ID: "speech@r1", Dim: 3, Metric: "cosine"}
	insertSpace(t, ctx, pool, space)
	old := []TextSpan{{Span: timeline.Span{StartMs: 0, EndMs: 10}, Text: "旧句子", Words: []Word{{W: "旧句子", S: 0, E: 10}}}}
	if err := db.WithTx(ctx, pool, func(tx pgx.Tx) error { return ReplaceSpeech(ctx, tx, w1, v1, "old-model", old) }); err != nil {
		t.Fatal(err)
	}
	vectors := []VectorRow{
		{v1, w1, "speech", space.ID, "sentence@v1", old[0].Span, []float32{1, 0, 0}},
		{v1, w1, "visual", space.ID, "grid@v1", old[0].Span, []float32{1, 0, 0}},
	}
	if err := UpsertVectors(ctx, pool, vectors); err != nil {
		t.Fatal(err)
	}
	duplicate := []TextSpan{{Span: old[0].Span, Text: "new"}, {Span: old[0].Span, Text: "duplicate"}}
	if err := db.WithTx(ctx, pool, func(tx pgx.Tx) error { return ReplaceSpeech(ctx, tx, w1, v1, "new-model", duplicate) }); err == nil {
		t.Fatal("duplicate sentences unexpectedly succeeded")
	}
	assertOldSpeech := func() {
		t.Helper()
		sents, err := SpeechSentences(ctx, pool, v1)
		if err != nil || len(sents) != 1 || sents[0].Text != "旧句子" || !reflect.DeepEqual(sents[0].Words, old[0].Words) {
			t.Fatalf("rollback sentences=%+v error=%v", sents, err)
		}
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM embedding WHERE video_id=$1", v1).Scan(&count); err != nil || count != 2 {
			t.Fatalf("rollback vectors=%d error=%v", count, err)
		}
	}
	assertOldSpeech()
	newSents := []TextSpan{
		{Span: timeline.Span{StartMs: 0, EndMs: 10}, Text: "第一句", Words: []Word{}},
		{Span: timeline.Span{StartMs: 10, EndMs: 20}, Text: "第二句"},
		{Span: timeline.Span{StartMs: 20, EndMs: 30}, Text: "第三句"},
	}
	sentinel := errors.New("stage update failed")
	err := db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		if err := ReplaceSpeech(ctx, tx, w1, v1, "new-model", newSents); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("caller error=%v", err)
	}
	assertOldSpeech()
	err = db.WithTx(ctx, pool, func(tx pgx.Tx) error { return ReplaceSpeech(ctx, tx, w2, v1, "new-model", nil) })
	if apperr.KindOf(err) != apperr.NotFound {
		t.Fatalf("foreign workspace error=%v", err)
	}
	assertOldSpeech()
	if err := db.WithTx(ctx, pool, func(tx pgx.Tx) error { return ReplaceSpeech(ctx, tx, w1, v1, "new-model", newSents) }); err != nil {
		t.Fatal(err)
	}
	sents, err := SpeechSentences(ctx, pool, v1)
	if err != nil || len(sents) != 3 || sents[0].Words == nil || sents[1].Words != nil || sents[0].Model != "new-model" || sents[0].VideoID != v1 || sents[0].WorkspaceID != w1 {
		t.Fatalf("replacement sentences=%+v error=%v", sents, err)
	}
	var visual, speech int
	if err := pool.QueryRow(ctx, "SELECT count(*) FILTER(WHERE channel='visual'),count(*) FILTER(WHERE channel='speech') FROM embedding WHERE video_id=$1", v1).Scan(&visual, &speech); err != nil || visual != 1 || speech != 0 {
		t.Fatalf("replacement visual=%d speech=%d error=%v", visual, speech, err)
	}
	span := timeline.Span{StartMs: 10, EndMs: 20}
	if got, err := TextRange(ctx, pool, w1, v1, span); err != nil || len(got) != 1 || got[0].Text != "第二句" {
		t.Fatalf("half-open text=%+v error=%v", got, err)
	}
	if got, err := TextRange(ctx, pool, w2, v1, span); err != nil || len(got) != 0 {
		t.Fatalf("foreign text=%+v error=%v", got, err)
	}
	if err := db.WithTx(ctx, pool, func(tx pgx.Tx) error { return ReplaceSpeech(ctx, tx, w1, v2, "indexing-model", newSents) }); err != nil {
		t.Fatal(err)
	}
	if got, err := SpeechSentences(ctx, pool, v2); err != nil || len(got) != 3 {
		t.Fatalf("indexing sentences unavailable to embedding: %+v error=%v", got, err)
	}
	if err := DeleteVideoChannel(ctx, pool, v1, "visual"); err != nil {
		t.Fatal(err)
	}
	if got, err := SpeechSentences(ctx, pool, v1); err != nil || len(got) != 3 {
		t.Fatalf("channel deletion removed text: %+v error=%v", got, err)
	}
	if err := DeleteVideo(ctx, pool, v1); err != nil {
		t.Fatal(err)
	}
	if got, err := SpeechSentences(ctx, pool, v1); err != nil || len(got) != 0 {
		t.Fatalf("video deletion left text: %+v error=%v", got, err)
	}
	if got, err := SpeechSentences(ctx, pool, v2); err != nil || len(got) != 3 {
		t.Fatalf("other video text changed: %+v error=%v", got, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE video SET deleted_at=now() WHERE id=$1", v2); err != nil {
		t.Fatal(err)
	}
	if got, err := SpeechSentences(ctx, pool, v2); err != nil || len(got) != 0 {
		t.Fatalf("deleted text exposed: %+v error=%v", got, err)
	}
}

func TestCleanupPostgresIntegration(t *testing.T) {
	ctx, pool := indexTestDB(t)
	w1 := insertWorkspace(t, ctx, pool, "first")
	w2 := insertWorkspace(t, ctx, pool, "second")
	v1 := insertVideo(t, ctx, pool, w1, "ready", false)
	v2 := insertVideo(t, ctx, pool, w2, "ready", false)
	v3 := insertVideo(t, ctx, pool, w1, "indexing", false)
	v4 := insertVideo(t, ctx, pool, w1, "ready", true)
	drop := VectorSpace{ID: "drop@r1", Dim: 3, Metric: "cosine"}
	keep := VectorSpace{ID: "keep@r1", Dim: 3, Metric: "cosine"}
	insertSpace(t, ctx, pool, drop)
	insertSpace(t, ctx, pool, keep)
	var rows []VectorRow
	for i, video := range []int64{v1, v2, v3, v4} {
		workspace := w1
		if i == 1 {
			workspace = w2
		}
		for _, sp := range []VectorSpace{drop, keep} {
			rows = append(rows, VectorRow{video, workspace, "visual", sp.ID, "grid@v1", timeline.Span{StartMs: 0, EndMs: 10}, []float32{1, 0, 0}})
		}
	}
	rows = append(rows,
		VectorRow{v1, w1, "visual", drop.ID, "grid@v1", timeline.Span{StartMs: 10, EndMs: 20}, []float32{1, 0, 0}},
		VectorRow{v1, w1, "speech", drop.ID, "sentence@v1", timeline.Span{StartMs: 0, EndMs: 10}, []float32{1, 0, 0}},
	)
	if err := UpsertVectors(ctx, pool, rows); err != nil {
		t.Fatal(err)
	}
	if n, err := Coverage(ctx, pool, "visual", drop.ID); err != nil || n != 2 {
		t.Fatalf("ready distinct coverage=%d error=%v", n, err)
	}
	for _, channel := range []string{"visual", "speech"} {
		if err := EnsureVectorIndex(ctx, pool, channel, drop); err != nil {
			t.Fatal(err)
		}
	}
	paused := pauseSpaceQuery{Querier: pool, scanned: make(chan struct{}), proceed: make(chan struct{})}
	writeErr := make(chan error, 1)
	go func() { writeErr <- UpsertVectors(ctx, paused, rows[:1]) }()
	select {
	case <-paused.scanned:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := DropSpace(ctx, pool, drop.ID, 1); err != nil {
		close(paused.proceed)
		t.Fatal(err)
	}
	close(paused.proceed)
	if err := <-writeErr; apperr.KindOf(err) != apperr.Unavailable {
		t.Fatalf("stale space check allowed dropped-space write: %v", err)
	}
	if n, err := Coverage(ctx, pool, "visual", drop.ID); err != nil || n != 0 {
		t.Fatalf("dropped coverage=%d error=%v", n, err)
	}
	for _, channel := range []string{"visual", "speech"} {
		if ready, err := VectorIndexReady(ctx, pool, channel, drop.ID); err != nil || ready {
			t.Fatalf("dropped index ready=%v error=%v", ready, err)
		}
	}
	var disabled bool
	if err := pool.QueryRow(ctx, "SELECT disabled FROM embedding_space WHERE id=$1", drop.ID).Scan(&disabled); err != nil || !disabled {
		t.Fatalf("space disabled=%v error=%v", disabled, err)
	}
	if err := UpsertVectors(ctx, pool, rows[:1]); apperr.KindOf(err) != apperr.Unavailable {
		t.Fatalf("disabled write error=%v", err)
	}
	if n, err := Coverage(ctx, pool, "visual", keep.ID); err != nil || n != 2 {
		t.Fatalf("other space coverage=%d error=%v", n, err)
	}
	if err := DeleteVideo(ctx, pool, v1); err != nil {
		t.Fatal(err)
	}
	if n, err := Coverage(ctx, pool, "visual", keep.ID); err != nil || n != 1 {
		t.Fatalf("scoped video cleanup coverage=%d error=%v", n, err)
	}
}

type pauseSpaceQuery struct {
	db.Querier
	scanned, proceed chan struct{}
}

func (q pauseSpaceQuery) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	row := q.Querier.QueryRow(ctx, sql, args...)
	if strings.HasPrefix(sql, "SELECT dim, metric") {
		return pausedRow{Row: row, scanned: q.scanned, proceed: q.proceed, ctx: ctx}
	}
	return row
}

type pausedRow struct {
	pgx.Row
	scanned, proceed chan struct{}
	ctx              context.Context
}

func (r pausedRow) Scan(dest ...any) error {
	err := r.Row.Scan(dest...)
	close(r.scanned)
	select {
	case <-r.proceed:
		return err
	case <-r.ctx.Done():
		return r.ctx.Err()
	}
}
