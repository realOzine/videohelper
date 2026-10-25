package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"math"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/pipeline"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/internal/timeline"
	"github.com/pgvector/pgvector-go"
)

type videoEvidence struct {
	ID                 int64           `json:"id"`
	WorkspaceID        int64           `json:"workspace_id"`
	Title              string          `json:"title"`
	SourceKind         string          `json:"source_kind"`
	SourceURL          *string         `json:"source_url"`
	SourceKey          *string         `json:"source_key"`
	NormalizedVideoKey *string         `json:"normalized_video_key"`
	DurationMs         *int            `json:"duration_ms"`
	HasVideo           *bool           `json:"has_video"`
	HasAudio           *bool           `json:"has_audio"`
	Status             string          `json:"status"`
	Deleted            bool            `json:"deleted"`
	Thumbs             any             `json:"thumbs"`
	Stages             []stageEvidence `json:"stages"`
}
type stageEvidence struct {
	Stage    string          `json:"stage"`
	Version  string          `json:"version"`
	Status   string          `json:"status"`
	Required bool            `json:"required"`
	Plan     json.RawMessage `json:"plan"`
}
type spaceEvidence struct {
	ID          string          `json:"id"`
	Dim         int             `json:"dim"`
	Metric      string          `json:"metric"`
	Fingerprint string          `json:"fingerprint"`
	DocInputs   []string        `json:"doc_inputs"`
	QueryInputs []string        `json:"query_inputs"`
	MaxSpanMs   *int            `json:"max_span_ms"`
	Disabled    bool            `json:"disabled"`
	Indexes     map[string]bool `json:"indexes"`
}
type textEvidence struct {
	VideoID     int64  `json:"video_id"`
	WorkspaceID int64  `json:"workspace_id"`
	Kind        string `json:"kind"`
	StartMs     int    `json:"start_ms"`
	EndMs       int    `json:"end_ms"`
	Text        string `json:"text"`
	Words       any    `json:"words"`
	Model       string `json:"model"`
}
type inspection struct {
	Passed      bool            `json:"passed"`
	Violations  []string        `json:"violations"`
	IndexSHA256 string          `json:"index_sha256"`
	Videos      []videoEvidence `json:"videos"`
	Spaces      []spaceEvidence `json:"spaces"`
	TextSpans   []textEvidence  `json:"text_spans"`
}

func jsonRows(ctx context.Context, tx pgx.Tx, sql string, dst any, args ...any) error {
	var raw []byte
	if err := tx.QueryRow(ctx, sql, args...).Scan(&raw); err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}

func inspect(ctx context.Context, pool *pgxpool.Pool, client *mlclient.Client, channels *space.Channels, r request) (inspection, error) {
	out := inspection{Violations: []string{}, Videos: []videoEvidence{}, Spaces: []spaceEvidence{}, TextSpans: []textEvidence{}}
	discovery := client.Discover(ctx)
	for endpoint, err := range discovery.Errors {
		out.Violations = append(out.Violations, fmt.Sprintf("ML discovery %s: %v", endpoint, err))
	}
	models := map[string][]mlclient.SpaceInfo{}
	for _, endpoint := range discovery.Endpoints {
		for _, sp := range endpoint.Spaces {
			models[sp.ID] = append(models[sp.ID], sp)
		}
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	var member bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM membership WHERE workspace_id=$1 AND user_id=$2)", r.WorkspaceID, r.UserID).Scan(&member); err != nil {
		return out, err
	}
	if !member {
		out.Violations = append(out.Violations, "user is not a workspace member")
	}
	err = jsonRows(ctx, tx, `SELECT COALESCE(jsonb_agg(to_jsonb(v) ORDER BY v.id),'[]') FROM (
SELECT id,workspace_id,title,source_kind,source_url,source_key,normalized_video_key,duration_ms,has_video,has_audio,status,
deleted_at IS NOT NULL AS deleted,thumbs,COALESCE((SELECT jsonb_agg(jsonb_build_object('stage',stage,'version',version,'status',status,'required',required,'plan',output->'plan') ORDER BY stage,version) FROM stage_run WHERE video_id=video.id),'[]') AS stages
FROM video WHERE id=ANY($1)) v`, &out.Videos, r.VideoIDs)
	if err != nil {
		return out, err
	}
	err = jsonRows(ctx, tx, `SELECT COALESCE(jsonb_agg(to_jsonb(t) ORDER BY t.video_id,t.kind,t.start_ms),'[]') FROM
(SELECT video_id,workspace_id,kind,start_ms,end_ms,text,words,model FROM text_span WHERE video_id=ANY($1)) t`, &out.TextSpans, r.VideoIDs)
	if err != nil {
		return out, err
	}
	spaceIDs := []string{}
	for _, ch := range channels.List() {
		spaceIDs = append(spaceIDs, ch.Active...)
	}
	// Include stored spaces too so unexpected or stale vectors change the content identity.
	err = jsonRows(ctx, tx, `SELECT COALESCE(jsonb_agg(to_jsonb(s) ORDER BY s.id),'[]') FROM
(SELECT id,dim,metric,fingerprint,doc_inputs,query_inputs,max_span_ms,disabled FROM embedding_space
WHERE id=ANY($1) OR id IN (SELECT space_id FROM embedding WHERE video_id=ANY($2))) s`, &out.Spaces, spaceIDs, r.VideoIDs)
	if err != nil {
		return out, err
	}
	spaces := map[string]*spaceEvidence{}
	for i := range out.Spaces {
		sp := &out.Spaces[i]
		sp.Indexes = map[string]bool{}
		spaces[sp.ID] = sp
	}
	for _, ch := range channels.List() {
		for _, id := range ch.Active {
			sp, ok := spaces[id]
			if !ok {
				out.Violations = append(out.Violations, "missing space "+id)
				continue
			}
			if sp.Disabled || sp.Dim < 1 || sp.Dim > 4000 || sp.Fingerprint == "" || !slices.Contains(sp.DocInputs, ch.Doc) || !slices.Contains(sp.QueryInputs, "text") {
				out.Violations = append(out.Violations, "invalid space "+id)
			}
			if (ch.Name == "visual" || ch.Name == "audio") && (sp.MaxSpanMs == nil || *sp.MaxSpanMs < 2500) {
				out.Violations = append(out.Violations, "space cannot encode grid: "+id)
			}
			if len(models[id]) == 0 {
				out.Violations = append(out.Violations, "model unavailable: "+id)
			}
			for _, model := range models[id] {
				if model.Dim != sp.Dim || model.Metric != sp.Metric || model.Fingerprint != sp.Fingerprint {
					out.Violations = append(out.Violations, "model identity mismatch: "+id)
				}
			}
			ready, err := index.VectorIndexReady(ctx, tx, ch.Name, id)
			if err != nil {
				return out, err
			}
			if ready {
				ready, err = validHNSW(ctx, tx, ch.Name, *sp)
				if err != nil {
					return out, err
				}
			}
			sp.Indexes[ch.Name] = ready
			if !ready {
				out.Violations = append(out.Violations, "HNSW unavailable: "+ch.Name+"/"+id)
			}
		}
	}
	validateVideos(&out, r, channels, spaces)
	digest := sha256.New()
	if err := hashJSON(digest, struct {
		WorkspaceID int64
		Videos      []videoEvidence
		Spaces      []spaceEvidence
		Text        []textEvidence
	}{r.WorkspaceID, out.Videos, out.Spaces, out.TextSpans}); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT video_id,workspace_id,channel,space_id,seg,start_ms,end_ms,vec FROM embedding WHERE video_id=ANY($1) ORDER BY video_id,channel,space_id,seg,start_ms,end_ms`, r.VideoIDs)
	if err != nil {
		return out, err
	}
	actual := map[vectorKey][]timeline.Span{}
	for rows.Next() {
		var videoID, workspaceID int64
		var channel, spaceID, seg string
		var span timeline.Span
		var vec pgvector.HalfVector
		if err := rows.Scan(&videoID, &workspaceID, &channel, &spaceID, &seg, &span.StartMs, &span.EndMs, &vec); err != nil {
			rows.Close()
			return out, err
		}
		key := vectorKey{videoID, channel, spaceID, seg}
		actual[key] = append(actual[key], span)
		if workspaceID != r.WorkspaceID {
			out.Violations = append(out.Violations, fmt.Sprintf("vector workspace mismatch: %v", key))
		}
		sp, ok := spaces[spaceID]
		if !ok || !validVector(vec.Slice(), sp.Dim) {
			out.Violations = append(out.Violations, fmt.Sprintf("invalid vector: %v at %d", key, span.StartMs))
		}
		if err := hashJSON(digest, []any{videoID, workspaceID, channel, spaceID, seg, span.StartMs, span.EndMs, len(vec.Slice())}); err != nil {
			rows.Close()
			return out, err
		}
		for _, value := range vec.Slice() {
			var raw [4]byte
			binary.LittleEndian.PutUint32(raw[:], math.Float32bits(value))
			digest.Write(raw[:])
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	validateCoverage(&out, channels, actual)
	if err := tx.Commit(ctx); err != nil {
		return out, err
	}
	slices.Sort(out.Violations)
	out.Violations = slices.Compact(out.Violations)
	out.Passed = len(out.Violations) == 0
	out.IndexSHA256 = hex.EncodeToString(digest.Sum(nil))
	return out, nil
}

func validHNSW(ctx context.Context, tx pgx.Tx, channel string, sp spaceEvidence) (bool, error) {
	rows, err := tx.Query(ctx, `SELECT pg_get_indexdef(i.indexrelid)||' '||o.opcname,pg_get_expr(i.indpred,i.indrelid)
FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid JOIN pg_am a ON a.oid=c.relam
JOIN pg_opclass o ON o.oid=i.indclass[0]
WHERE i.indrelid='embedding'::regclass AND i.indisvalid AND i.indisready AND a.amname='hnsw' AND i.indpred IS NOT NULL`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var def, predicate string
		if err := rows.Scan(&def, &predicate); err != nil {
			return false, err
		}
		if hnswMatches(def, predicate, channel, sp) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func hnswMatches(def, predicate, channel string, sp spaceEvidence) bool {
	class := map[string]string{"cosine": "halfvec_cosine_ops", "ip": "halfvec_ip_ops", "l2": "halfvec_l2_ops"}[sp.Metric]
	compact := strings.NewReplacer(" ", "", "(", "", ")", "").Replace(predicate)
	want := "channel='" + channel + "'::textANDspace_id='" + sp.ID + "'::text"
	return class != "" && strings.Contains(def, fmt.Sprintf("halfvec(%d)", sp.Dim)) && strings.Contains(def, class) &&
		compact == want
}

type vectorKey struct {
	VideoID               int64
	Channel, SpaceID, Seg string
}

func validVector(values []float32, dim int) bool {
	if dim <= 0 || len(values) != dim {
		return false
	}
	nonzero := false
	for _, v := range values {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
		nonzero = nonzero || v != 0
	}
	return nonzero
}

func hashJSON(h hash.Hash, value any) error {
	// Decode raw JSON before encoding so PostgreSQL JSON formatting cannot affect identity.
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var canonical any
	d := json.NewDecoder(strings.NewReader(string(raw)))
	d.UseNumber()
	if err := d.Decode(&canonical); err != nil {
		return err
	}
	return json.NewEncoder(h).Encode(canonical)
}

func validateVideos(out *inspection, r request, channels *space.Channels, spaces map[string]*spaceEvidence) {
	seen := map[int64]bool{}
	for _, v := range out.Videos {
		seen[v.ID] = true
		bad := func(reason string) {
			out.Violations = append(out.Violations, fmt.Sprintf("video %d: %s", v.ID, reason))
		}
		if v.WorkspaceID != r.WorkspaceID {
			bad("workspace mismatch")
		}
		if v.Status != "ready" || v.Deleted {
			bad("not ready or deleted")
		}
		if v.DurationMs == nil || *v.DurationMs <= 0 || v.HasVideo == nil || v.HasAudio == nil || v.SourceKey == nil || *v.SourceKey == "" || v.NormalizedVideoKey == nil || *v.NormalizedVideoKey == "" {
			bad("incomplete media metadata")
		}
		required := map[string]bool{"probe": false, "prepare_media": false, "finalize": false}
		if v.HasAudio != nil && *v.HasAudio {
			required["asr"] = false
		}
		for _, ch := range channels.List() {
			if eligible(v, ch.Name) {
				for _, id := range ch.Active {
					required["embed:"+ch.Name+":"+id] = false
				}
			}
		}
		for _, stage := range v.Stages {
			if stage.Required && stage.Status != "done" {
				bad("required stage incomplete: " + stage.Stage)
			}
			if _, ok := required[stage.Stage]; ok && stage.Required && stage.Status == "done" {
				required[stage.Stage] = true
			}
			if strings.HasPrefix(stage.Stage, "embed:") && stage.Required {
				var plan pipeline.StageDef
				if err := json.Unmarshal(stage.Plan, &plan); err != nil {
					bad("invalid embedding plan: " + stage.Stage)
					continue
				}
				sp, ok := spaces[plan.Options.Space.ID]
				if !ok || plan.Name != stage.Stage || plan.Version != stage.Version || plan.Options.Space.Dim != sp.Dim || plan.Options.Space.Fingerprint != sp.Fingerprint || plan.Options.Space.Metric != sp.Metric {
					bad("stage space identity mismatch: " + stage.Stage)
				}
			}
		}
		for name, done := range required {
			if !done {
				bad("missing completed required stage: " + name)
			}
		}
		count := 0
		for _, text := range out.TextSpans {
			if text.VideoID != v.ID {
				continue
			}
			count++
			if text.WorkspaceID != r.WorkspaceID || strings.TrimSpace(text.Text) == "" || text.Model == "" || text.StartMs < 0 || text.EndMs <= text.StartMs || v.DurationMs == nil || text.EndMs > *v.DurationMs {
				bad("invalid ASR text span")
			}
		}
		if v.HasAudio != nil && *v.HasAudio && count == 0 {
			bad("empty ASR")
		}
	}
	for _, id := range r.VideoIDs {
		if !seen[id] {
			out.Violations = append(out.Violations, fmt.Sprintf("missing video %d", id))
		}
	}
}

func eligible(v videoEvidence, channel string) bool {
	if channel == "visual" {
		return v.HasVideo != nil && *v.HasVideo
	}
	return v.HasAudio != nil && *v.HasAudio
}

func validateCoverage(out *inspection, channels *space.Channels, actual map[vectorKey][]timeline.Span) {
	expected := map[vectorKey][]timeline.Span{}
	for _, v := range out.Videos {
		if v.DurationMs == nil || *v.DurationMs <= 0 {
			continue
		}
		original := []timeline.Sentence{}
		for _, t := range out.TextSpans {
			if t.VideoID == v.ID && t.Kind == "speech" {
				original = append(original, timeline.Sentence{Span: timeline.Span{StartMs: t.StartMs, EndMs: t.EndMs}, Text: t.Text})
			}
		}
		speech := []timeline.Span{}
		for _, s := range timeline.MergeSentences(original, 8, 120) {
			speech = append(speech, s.Span)
		}
		for _, ch := range channels.List() {
			if !eligible(v, ch.Name) {
				continue
			}
			spans := timeline.Cells(*v.DurationMs)
			if ch.Name == "speech" {
				spans = speech
			}
			for _, id := range ch.Active {
				expected[vectorKey{v.ID, ch.Name, id, ch.Seg}] = spans
			}
		}
	}
	for key, want := range expected {
		if !slices.Equal(want, actual[key]) {
			out.Violations = append(out.Violations, fmt.Sprintf("vector interval coverage mismatch: %v (want %d, got %d)", key, len(want), len(actual[key])))
		}
	}
	for key := range actual {
		if _, ok := expected[key]; !ok {
			out.Violations = append(out.Violations, fmt.Sprintf("unexpected vector route: %v", key))
		}
	}
}
