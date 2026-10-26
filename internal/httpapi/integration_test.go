package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-jose/go-jose/v4"
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ozine/videohelper/internal/config"
	"github.com/ozine/videohelper/internal/index"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/mlclient"
	"github.com/ozine/videohelper/internal/pipeline"
	"github.com/ozine/videohelper/internal/platform/authn"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/platform/db"
	"github.com/ozine/videohelper/internal/search"
	"github.com/ozine/videohelper/internal/space"
	"github.com/ozine/videohelper/internal/timeline"
	"github.com/ozine/videohelper/internal/workspace"
	"github.com/ozine/videohelper/migrations"
	"github.com/redis/go-redis/v9"
)

func TestCoreHTTPIntegration(t *testing.T) {
	dsn, s3endpoint, redisAddr := os.Getenv("VH_TEST_HTTPAPI_DB_URL"), os.Getenv("VH_TEST_S3_ENDPOINT"), os.Getenv("VH_TEST_REDIS_ADDR")
	if dsn == "" || s3endpoint == "" || redisAddr == "" {
		t.Skip("set dedicated VH_TEST_HTTPAPI_DB_URL, local VH_TEST_S3_ENDPOINT and VH_TEST_REDIS_ADDR (empty Redis DB12)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sqldb, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sqldb.Close()
	var dbname string
	var count int
	if err := sqldb.QueryRowContext(ctx, "SELECT current_database()").Scan(&dbname); err != nil || dbname != "videohelper_httpapi_test" {
		t.Fatalf("refusing database %q: %v", dbname, err)
	}
	if err := sqldb.QueryRowContext(ctx, "SELECT count(*) FROM pg_tables WHERE schemaname='public' AND tablename<>'goose_db_version'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("refusing nonempty database: %d %v", count, err)
	}
	if err := migrations.Up(ctx, sqldb); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if err := migrations.DownTo(cleanup, sqldb, 0); err != nil {
			t.Error(err)
		}
	}()
	pool, err := db.Open(ctx, dsn, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	rdb := redis.NewClient(&redis.Options{Addr: redisAddr, DB: 12})
	defer rdb.Close()
	if size, err := rdb.DBSize(ctx).Result(); err != nil || size != 0 {
		t.Fatalf("refusing nonempty dedicated Redis DB12: %d %v", size, err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := rdb.FlushDB(cleanup).Err(); err != nil {
			t.Error(err)
		}
	}()
	queueOpt := asynq.RedisClientOpt{Addr: redisAddr, DB: 12}
	queueClient, inspector := asynq.NewClient(queueOpt), asynq.NewInspector(queueOpt)
	defer queueClient.Close()
	defer inspector.Close()
	t.Setenv("AWS_ACCESS_KEY_ID", "videohelper-local")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "videohelper-local-secret")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-1"))
	if err != nil {
		t.Fatal(err)
	}
	s3client := s3.NewFromConfig(awsCfg, func(o *s3.Options) { o.BaseEndpoint = aws.String(s3endpoint); o.UsePathStyle = true })
	bucket := fmt.Sprintf("videohelper-httpapi-%d", time.Now().UnixNano())
	objects := blob.New(s3client, bucket)
	if err := objects.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		uploads, err := s3client.ListMultipartUploads(cleanup, &s3.ListMultipartUploadsInput{Bucket: aws.String(bucket)})
		if err != nil {
			t.Error(err)
		} else {
			for _, upload := range uploads.Uploads {
				if err := objects.AbortMultipart(cleanup, aws.ToString(upload.Key), aws.ToString(upload.UploadId)); err != nil {
					t.Error(err)
				}
			}
		}
		if err := objects.DeletePrefix(cleanup, "workspaces/"); err != nil {
			t.Error(err)
		}
		if _, err := s3client.DeleteBucket(cleanup, &s3.DeleteBucketInput{Bucket: aws.String(bucket)}); err != nil {
			t.Error(err)
		}
	}()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "http-test", Algorithm: string(jose.RS256), Use: "sig"}}})
	}))
	defer jwks.Close()
	verifier, err := authn.New(ctx, config.AuthConfig{Issuer: jwks.URL, JWKSURL: jwks.URL + "/jwks", Audience: "videohelper-httpapi"})
	if err != nil {
		t.Fatal(err)
	}
	token := func(subject string) string {
		t.Helper()
		signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "http-test"}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(map[string]any{"iss": jwks.URL, "aud": "videohelper-httpapi", "sub": subject, "exp": time.Now().Add(time.Hour).Unix(), "email": subject + "@example.test"})
		signed, err := signer.Sign(body)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := signed.CompactSerialize()
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	var embeds atomic.Int32
	var encodedID atomic.Value
	encodedID.Store("")
	var encodedSpace atomic.Value
	encodedSpace.Store("")
	encodingStarted, encodingCanceled := make(chan struct{}), make(chan struct{})
	fingerprint := "sha256:" + strings.Repeat("a", 64)
	maxSpan := 30000
	metadata := mlclient.SpacesResponse{Spaces: []mlclient.SpaceInfo{{ID: "http-contract@r1", Dim: 3, Metric: "cosine", QueryInputs: []string{"text"}, DocInputs: []string{"video", "text", "image"}, MaxSpanMs: &maxSpan, MaxBatch: 8, Fingerprint: fingerprint}}, Transcribers: []mlclient.TranscriberInfo{}}
	metadata.Spaces = append(metadata.Spaces, mlclient.SpaceInfo{ID: "http-choice@r1", Dim: 3, Metric: "cosine", QueryInputs: []string{"text"}, DocInputs: []string{"video", "audio"}, MaxSpanMs: &maxSpan, MaxBatch: 8, Fingerprint: fingerprint})
	mlserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/spaces" {
			json.NewEncoder(w).Encode(metadata)
			return
		}
		embeds.Add(1)
		encodedID.Store(r.Header.Get("X-Request-ID"))
		var req mlclient.EmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Role != "query" || len(req.Inputs) != 1 || req.Inputs[0].Text == "" {
			t.Errorf("HTTP search did not encode text: %+v %v", req, err)
			w.WriteHeader(400)
			return
		}
		if req.Inputs[0].Text == "等待取消" {
			close(encodingStarted)
			<-r.Context().Done()
			close(encodingCanceled)
			return
		}
		encodedSpace.Store(req.Space)
		json.NewEncoder(w).Encode(mlclient.EmbedResponse{Space: req.Space, Fingerprint: fingerprint, Dim: 3, Vectors: [][]float32{{1, 0, 0}}})
	}))
	defer mlserver.Close()
	var registry *space.Registry
	ml := mlclient.New(config.MLConfig{Endpoints: []string{mlserver.URL}, Token: "explicit-http-contract-test"}, func(id string) (string, bool) { return registry.Resolve(id) })
	registry = space.New(pool, ml, config.MLConfig{})
	if err := registry.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	channels, err := space.NewChannels(config.ChannelsConfig{"visual": {Doc: "video", Seg: timeline.GridSeg, Weight: 1, Active: []string{"http-contract@r1"}, Shadow: []string{"http-choice@r1"}}, "speech": {Doc: "text", Seg: timeline.SentenceSeg, Weight: 1, Active: []string{"http-contract@r1"}}, "audio": {Doc: "audio", Seg: timeline.GridSeg, Weight: 1, Active: []string{"http-choice@r1"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, channel := range channels.List() {
		for _, id := range append(channel.Active, channel.Shadow...) {
			if err := index.EnsureVectorIndex(ctx, pool, channel.Name, index.VectorSpace{ID: id, Dim: 3, Metric: "cosine"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	pl := pipeline.New(pipeline.Deps{Pool: pool, Client: queueClient, Inspector: inspector, Redis: rdb, Channels: channels, Registry: registry, Worker: config.WorkerConfig{MaxRetry: 2, WorkspaceGPULimit: 1}})
	lb := library.New(pool, objects, library.Config{}, pl, pl)
	se := search.New(pool, lb, registry, channels, ml, config.SearchConfig{K: 10, RRFK: 60, MergeGapCells: 1, MaxLimit: 50, ChannelWeights: map[string]float64{"audio": 0}})
	server := httptest.NewServer(New(Deps{Authn: verifier, Workspace: workspace.New(pool), Library: lb, Pipeline: pl, Search: se, Blob: objects, Ready: pool.Ping}))
	defer server.Close()
	owner, foreign := token("http-owner"), token("http-foreign")
	call := func(method, path, body, bearer string, headers map[string]string) (int, []byte) {
		t.Helper()
		request, err := http.NewRequestWithContext(ctx, method, server.URL+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if bearer != "" {
			request.Header.Set("Authorization", "Bearer "+bearer)
		}
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		if response.Header.Get("X-Request-ID") == "" {
			t.Fatal("missing response request ID")
		}
		return response.StatusCode, data
	}
	if status, _ := call("GET", "/v1/me", "", "", nil); status != 401 {
		t.Fatalf("missing Bearer status %d", status)
	}
	if status, body := call("GET", "/v1/me", "", "invalid.token.value", nil); status != 401 || strings.Contains(string(body), "invalid.token.value") {
		t.Fatalf("invalid Bearer response: %d %s", status, body)
	}
	status, body := call("GET", "/v1/me", "", owner, nil)
	if status != 200 {
		t.Fatalf("me: %d %s", status, body)
	}
	var me map[string]any
	json.Unmarshal(body, &me)
	workspaceID, ok := me["workspace_id"].(string)
	if !ok || workspaceID == "" {
		t.Fatalf("workspace ID not a string: %s", body)
	}
	if status, body := call("GET", "/v1/me", "", foreign, map[string]string{"X-Workspace-ID": workspaceID}); status != 403 {
		t.Fatalf("foreign workspace selection: %d %s", status, body)
	}
	if status, _ := call("GET", "/readyz", "", "", nil); status != 200 {
		t.Fatalf("configured readiness status %d", status)
	}
	status, body = call("POST", "/v1/projects", `{"name":"测试项目"}`, owner, nil)
	if status != 201 {
		t.Fatalf("create project: %d %s", status, body)
	}
	var project struct {
		Project projectResponse `json:"project"`
	}
	if err := json.Unmarshal(body, &project); err != nil {
		t.Fatal(err)
	}
	source := "explicit upload storage fixture"
	requestBody, _ := json.Marshal(map[string]any{"filename": "测试上传.mp4", "size_bytes": len(source), "content_type": "video/mp4", "project_id": project.Project.ID})
	status, body = call("POST", "/v1/uploads", string(requestBody), owner, nil)
	if status != 201 {
		t.Fatalf("start upload: %d %s", status, body)
	}
	var upload struct {
		Video  library.Video      `json:"video"`
		Upload library.UploadPlan `json:"upload"`
	}
	if err := json.Unmarshal(body, &upload); err != nil {
		t.Fatal(err)
	}
	if upload.Video.ID <= 0 || len(upload.Upload.Parts) != 1 || strings.Contains(string(body), "source_key") {
		t.Fatalf("upload response exposed storage or lacked IDs: %s", body)
	}
	videoID := strconv.FormatInt(upload.Video.ID, 10)
	put, err := http.NewRequestWithContext(ctx, "PUT", upload.Upload.Parts[0].URL, strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	putResponse, err := http.DefaultClient.Do(put)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, putResponse.Body)
	putResponse.Body.Close()
	if putResponse.StatusCode != 200 {
		t.Fatalf("signed part PUT: %s", putResponse.Status)
	}
	complete, _ := json.Marshal(map[string]any{"upload_id": upload.Upload.UploadID, "parts": []blob.Part{{N: 1, ETag: putResponse.Header.Get("ETag")}}})
	if status, body := call("POST", "/v1/videos/"+videoID+"/complete", string(complete), owner, nil); status != 202 {
		t.Fatalf("complete upload: %d %s", status, body)
	}
	status, body = call("GET", "/v1/videos/"+videoID, "", owner, nil)
	var detail struct {
		Stages []pipeline.StageStatus `json:"stages"`
	}
	if err := json.Unmarshal(body, &detail); err != nil || status != 200 || len(detail.Stages) != 1 || detail.Stages[0].Stage != "probe" || detail.Stages[0].Version != "v1" || strings.Contains(string(body), "output") {
		t.Fatalf("public pipeline stages: %d %s %v", status, body, err)
	}
	if status, body := call("GET", "/v1/videos/"+videoID, "", foreign, nil); status != 404 {
		t.Fatalf("foreign video: %d %s", status, body)
	}
	for _, bad := range []string{"0", "-1", "abc", "9223372036854775808"} {
		if status, body := call("GET", "/v1/videos/"+bad, "", owner, nil); status != 400 {
			t.Fatalf("invalid path ID %q: %d %s", bad, status, body)
		}
	}
	if status, body := call("POST", "/v1/videos/"+videoID+"/retry", "", owner, nil); status != 409 {
		t.Fatalf("retry nonfailed video: %d %s", status, body)
	}
	if _, err := pool.Exec(ctx, "UPDATE video SET status='failed' WHERE id=$1;", upload.Video.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE stage_run SET status='failed' WHERE video_id=$1", upload.Video.ID); err != nil {
		t.Fatal(err)
	}
	if status, body := call("POST", "/v1/videos/"+videoID+"/retry", "", owner, nil); status != 202 {
		t.Fatalf("retry failed video: %d %s", status, body)
	}
	wsID, _ := strconv.ParseInt(workspaceID, 10, 64)
	normalized := blob.RunNormalizedVideoKey(wsID, upload.Video.ID, "v1.http-test", 3)
	if err := objects.Put(ctx, normalized, strings.NewReader("explicit normalized storage fixture"), "video/mp4"); err != nil {
		t.Fatal(err)
	}
	var sprite bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 32, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	if err := jpeg.Encode(&sprite, img, nil); err != nil {
		t.Fatal(err)
	}
	spriteKey := blob.RunSpriteKey(wsID, upload.Video.ID, "v1.http-test", 3, 0)
	if err := objects.Put(ctx, spriteKey, bytes.NewReader(sprite.Bytes()), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	if err := library.UpdateProbe(ctx, pool, upload.Video.ID, 2000, true, true); err != nil {
		t.Fatal(err)
	}
	if err := library.UpdatePreparedMedia(ctx, pool, upload.Video.ID, normalized, timeline.SpriteLayout{TileW: 16, TileH: 8, Cols: 2, Rows: 1, Count: 2}); err != nil {
		t.Fatal(err)
	}
	original := "这里是未经改写的中文原话。"
	if err := db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		return index.ReplaceSpeech(ctx, tx, wsID, upload.Video.ID, "http-test-asr", []index.TextSpan{{Span: timeline.Span{StartMs: 1000, EndMs: 2000}, Text: original}})
	}); err != nil {
		t.Fatal(err)
	}
	var vectors []index.VectorRow
	for _, channel := range channels.List() {
		for _, id := range append(channel.Active, channel.Shadow...) {
			vectors = append(vectors, index.VectorRow{VideoID: upload.Video.ID, WorkspaceID: wsID, Channel: channel.Name, SpaceID: id, Seg: channel.Seg, Span: timeline.Span{StartMs: 1000, EndMs: 2000}, Vec: []float32{1, 0, 0}})
		}
	}
	if err := index.UpsertVectors(ctx, pool, vectors); err != nil {
		t.Fatal(err)
	}
	if err := library.SetStatus(ctx, pool, upload.Video.ID, "ready", ""); err != nil {
		t.Fatal(err)
	}
	before := embeds.Load()
	status, body = call("POST", "/v1/search", `{"query":"测试中文"}`, owner, map[string]string{"X-Request-ID": "http-evidence-id"})
	if status != 200 || embeds.Load()-before != 1 || encodedID.Load().(string) != "http-evidence-id" {
		t.Fatalf("search encoding/trace: %d %s", status, body)
	}
	var found struct {
		Moments []momentResponse `json:"moments"`
		Skipped []string         `json:"skipped_channels"`
	}
	if err := json.Unmarshal(body, &found); err != nil {
		t.Fatal(err)
	}
	if len(found.Moments) != 1 || found.Moments[0].VideoID != videoID || found.Moments[0].VideoTitle != "测试上传.mp4" || found.Moments[0].Snippet == nil || *found.Moments[0].Snippet != original || found.Moments[0].Thumbnail == nil {
		t.Fatalf("search DTO: %s", body)
	}
	thumb := found.Moments[0].Thumbnail
	parsed, _ := url.Parse(thumb.URL)
	if thumb.X != 16 || thumb.Y != 0 || thumb.W != 16 || thumb.H != 8 || parsed.Path != "/"+bucket+"/"+spriteKey {
		t.Fatalf("thumbnail used wrong immutable run or tile: %+v", thumb)
	}
	get, err := http.Get(thumb.URL)
	if err != nil {
		t.Fatal(err)
	}
	imageBytes, _ := io.ReadAll(get.Body)
	get.Body.Close()
	if get.StatusCode != 200 || !bytes.Equal(imageBytes, sprite.Bytes()) {
		t.Fatalf("thumbnail did not resolve persisted artifact: %s", get.Status)
	}
	status, body = call("GET", "/v1/videos/"+videoID, "", owner, nil)
	var posted struct {
		Video struct {
			Thumbnail *thumbnail `json:"thumbnail"`
		} `json:"video"`
	}
	if err := json.Unmarshal(body, &posted); status != 200 || err != nil || posted.Video.Thumbnail == nil {
		t.Fatalf("video poster: %d %s", status, body)
	}
	if poster, _ := url.Parse(posted.Video.Thumbnail.URL); posted.Video.Thumbnail.X != 16 || poster.Path != "/"+bucket+"/"+spriteKey {
		t.Fatalf("video poster used wrong tile: %+v", posted.Video.Thumbnail)
	}
	for _, invalid := range []string{`{"query":"文本","image":{"uri":"s3://bucket/key"}}`, `{"query":"文本","video_ids":[1]}`, `{"query":"文本","project_id":1}`} {
		if status, body := call("POST", "/v1/search", invalid, owner, nil); status != 400 {
			t.Fatalf("invalid search accepted: %d %s", status, body)
		}
	}
	t.Run("model catalog and explicit selection", func(t *testing.T) {
		for _, tc := range []struct {
			body, bearer string
			headers      map[string]string
			want         int
		}{
			{body: `{}`, want: 401},
			{body: `{}`, bearer: foreign, headers: map[string]string{"X-Workspace-ID": workspaceID}, want: 403},
			{body: `{"video_ids":["` + videoID + `"]}`, bearer: foreign, want: 404},
			{body: `{"project_id":"` + project.Project.ID + `"}`, bearer: foreign, want: 404},
			{body: `{"project_id":"` + project.Project.ID + `","video_ids":[]}`, bearer: owner, want: 400},
		} {
			if status, body := call("POST", "/v1/search/models", tc.body, tc.bearer, tc.headers); status != tc.want {
				t.Fatalf("catalog authorization/scope: %d want %d: %s", status, tc.want, body)
			}
		}
		catalog := func(scope, bearer string, eligible int, ready bool) {
			t.Helper()
			before := embeds.Load()
			status, body := call("POST", "/v1/search/models", scope, bearer, nil)
			var result struct {
				Channels []search.ChannelModels `json:"channels"`
			}
			if err := json.Unmarshal(body, &result); status != 200 || err != nil || len(result.Channels) != 3 || embeds.Load() != before {
				t.Fatalf("catalog response/inference: %d %s %v", status, body, err)
			}
			if strings.Contains(string(body), mlserver.URL) || strings.Contains(string(body), "endpoint") || strings.Contains(string(body), "explicit-http-contract-test") {
				t.Fatalf("catalog leaked runtime configuration: %s", body)
			}
			for _, channel := range result.Channels {
				if channel.DocInput == "" || len(channel.DefaultSpaceIDs) != 1 || len(channel.Models) != 2 || (channel.Channel == "audio" && channel.DefaultWeight != 0) {
					t.Fatalf("catalog defaults/capabilities: %+v", channel)
				}
				for _, model := range channel.Models {
					compatible := !(channel.Channel == "audio" && model.SpaceID == "http-contract@r1" || channel.Channel == "speech" && model.SpaceID == "http-choice@r1")
					if len(model.DocInputs) == 0 || len(model.QueryInputs) != 1 || model.QueryInputs[0] != "text" {
						t.Fatalf("missing model capabilities: %+v", model)
					}
					if !compatible {
						if model.Status != "incompatible" {
							t.Fatalf("incompatible model offered: %+v", model)
						}
						continue
					}
					indexed := 0
					if ready {
						indexed = eligible
					}
					if model.EligibleVideos != eligible || model.IndexedVideos != indexed || (ready && model.Status != "ready") {
						t.Fatalf("scoped readiness: scope=%s model=%+v", scope, model)
					}
				}
			}
		}
		catalog(`{}`, owner, 1, false)
		catalog(`{"project_id":"`+project.Project.ID+`"}`, owner, 1, false)
		catalog(`{"video_ids":["`+videoID+`"]}`, owner, 1, false)
		catalog(`{"video_ids":[]}`, owner, 0, true)
		catalog(`{}`, foreign, 0, true)
		before := embeds.Load()
		if status, body := call("POST", "/v1/search", `{"query":"显式选择","space_overrides":{"visual":"http-choice@r1"}}`, owner, nil); status != 409 || !strings.Contains(string(body), `"code":"not_ready"`) || embeds.Load() != before {
			t.Fatalf("incomplete selection reached ML: %d %s", status, body)
		}
		for _, invalid := range []string{
			`{"space_overrides":{"unknown":"http-choice@r1"}}`, `{"space_overrides":{"visual":""}}`, `{"space_overrides":{"visual":" "}}`, `{"space_overrides":{"visual":"missing@r1"}}`, `{"space_overrides":{"speech":"http-choice@r1"}}`,
			`{"channel_weights":{"unknown":1}}`, `{"channel_weights":{"visual":-1}}`, `{"channel_weights":{"audio":"1"}}`, `{"space_overrides":{"visual":1}}`, `{"channel_weights":{"audio":1e999}}`,
		} {
			if status, body := call("POST", "/v1/search", `{"query":"选择验证",`+invalid[1:], owner, nil); status != 400 || embeds.Load() != before {
				t.Fatalf("invalid selection reached ML: %d %s", status, body)
			}
		}
		for _, channel := range channels.List() {
			for _, id := range append(channel.Active, channel.Shadow...) {
				info, _ := registry.Get(id)
				name := "embed:" + channel.Name + ":" + id
				output, err := json.Marshal(map[string]any{"plan": pipeline.StageDef{Name: name, Version: "v1", Options: pipeline.StageOptions{Channel: channel.Name, Seg: channel.Seg, Space: info}}})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := pool.Exec(ctx, "INSERT INTO stage_run(video_id,stage,version,status,output) VALUES($1,$2,'http-test','done',$3)", upload.Video.ID, name, output); err != nil {
					t.Fatal(err)
				}
			}
		}
		catalog(`{}`, owner, 1, true)
		for _, tc := range []struct {
			body, channel, space string
		}{
			{`{"query":"显式选择","space_overrides":{"visual":"http-choice@r1"},"channel_weights":{"speech":0}}`, "visual", "http-choice@r1"},
			{`{"query":"音频保持关闭","space_overrides":{"audio":"http-choice@r1"},"channel_weights":{"speech":0}}`, "visual", "http-contract@r1"},
			{`{"query":"显式开启音频","space_overrides":{"audio":"http-choice@r1"},"channel_weights":{"visual":0,"speech":0,"audio":1}}`, "audio", "http-choice@r1"},
		} {
			before := embeds.Load()
			status, body := call("POST", "/v1/search", tc.body, owner, nil)
			if err := json.Unmarshal(body, &found); status != 200 || err != nil || embeds.Load()-before != 1 || encodedSpace.Load() != tc.space || len(found.Moments) != 1 || len(found.Moments[0].Channels) != 1 || found.Moments[0].Channels[0].Channel != tc.channel || len(found.Skipped) != 0 {
				t.Fatalf("selected route/weights: %d %s encoded=%v %v", status, body, encodedSpace.Load(), err)
			}
		}
	})
	t.Run("caller cancellation reaches ML", func(t *testing.T) {
		callCtx, stop := context.WithCancel(ctx)
		defer stop()
		request, err := http.NewRequestWithContext(callCtx, "POST", server.URL+"/v1/search", strings.NewReader(`{"query":"等待取消"}`))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Authorization", "Bearer "+owner)
		done := make(chan error, 1)
		go func() {
			response, err := http.DefaultClient.Do(request)
			if response != nil {
				response.Body.Close()
			}
			done <- err
		}()
		select {
		case <-encodingStarted:
		case <-time.After(time.Second):
			t.Fatal("ML request never started")
		}
		stop()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("HTTP caller cancellation: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("HTTP request did not cancel")
		}
		select {
		case <-encodingCanceled:
		case <-time.After(time.Second):
			t.Fatal("HTTP cancellation did not reach ML")
		}
	})
	status, body = call("GET", "/v1/videos/"+videoID+"/playback", "", owner, nil)
	if status != 200 {
		t.Fatalf("playback: %d %s", status, body)
	}
	var playback struct {
		URL     string    `json:"url"`
		Expires time.Time `json:"expires_at"`
	}
	json.Unmarshal(body, &playback)
	parsed, _ = url.Parse(playback.URL)
	if parsed.Path != "/"+bucket+"/"+normalized || playback.Expires.Location() != time.UTC {
		t.Fatalf("playback key/time: %+v", playback)
	}
	if status, body := call("GET", "/v1/projects", "", owner, nil); status != 200 || !strings.Contains(string(body), `"video_count":1`) {
		t.Fatalf("project counts: %d %s", status, body)
	}
	if status, body := call("DELETE", "/v1/projects/"+project.Project.ID+"/videos/"+videoID, "", owner, nil); status != 204 {
		t.Fatalf("remove project relation: %d %s", status, body)
	}
	if status, body := call("PUT", "/v1/projects/"+project.Project.ID+"/videos/"+videoID, "", owner, nil); status != 204 {
		t.Fatalf("add project relation: %d %s", status, body)
	}
	if status, body := call("GET", "/v1/videos?project_id="+project.Project.ID+"&limit=1", "", owner, nil); status != 200 || !strings.Contains(string(body), `"id":"`+videoID+`"`) || !strings.Contains(string(body), `"next_cursor":null`) {
		t.Fatalf("scoped video list DTO: %d %s", status, body)
	}
	if status, body := call("DELETE", "/v1/projects/"+project.Project.ID, "", owner, nil); status != 204 {
		t.Fatalf("delete project: %d %s", status, body)
	}
	if status, body := call("GET", "/v1/videos/"+videoID, "", owner, nil); status != 200 || strings.Contains(string(body), "normalized_video_key") || strings.Contains(string(body), "source_key") {
		t.Fatalf("video storage exposure: %d %s", status, body)
	}
	if status, body := call("DELETE", "/v1/videos/"+videoID, "", owner, nil); status != 202 {
		t.Fatalf("delete: %d %s", status, body)
	}
	if status, body := call("GET", "/v1/videos/"+videoID, "", owner, nil); status != 404 {
		t.Fatalf("deleted video visible: %d %s", status, body)
	}
	if status, body := call("POST", "/v1/videos/"+videoID+"/restore", "", owner, nil); status != 202 {
		t.Fatalf("restore: %d %s", status, body)
	}
}
