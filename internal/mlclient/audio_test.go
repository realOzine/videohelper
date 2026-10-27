package mlclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ozine/videohelper/internal/config"
)

func TestAudioWireAndWindowBoundaries(t *testing.T) {
	const id, text = "clap-htsat-unfused@r1", "有狗叫和关门的声音"
	span := 10000
	info := SpaceInfo{ID: id, Dim: 512, Metric: "cosine", QueryInputs: []string{"text"}, DocInputs: []string{"audio"}, MaxSpanMs: &span, MaxBatch: 8, Fingerprint: testFingerprint}
	received := make(chan EmbedRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/spaces" {
			writeJSON(w, SpacesResponse{Spaces: []SpaceInfo{info}, Transcribers: []TranscriberInfo{}})
			return
		}
		var req EmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		received <- req
		vector := make([]float32, info.Dim)
		vector[0] = 1
		writeJSON(w, EmbedResponse{Space: id, Fingerprint: testFingerprint, Dim: info.Dim, Vectors: [][]float32{vector}})
	}))
	defer server.Close()
	client := New(config.MLConfig{Token: "test", EmbedTimeout: time.Second, QueryTimeout: time.Second}, func(string) (string, bool) { return server.URL, true })
	reference := &MediaRef{URI: "local://media/normalized_video.mp4", StartMs: 1200, EndMs: 3700}
	if _, err := client.Embed(context.Background(), EmbedRequest{Space: id, Role: "document", Inputs: []EmbedInput{{Audio: reference}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Embed(context.Background(), EmbedRequest{Space: id, Role: "query", Inputs: []EmbedInput{{Text: text}}}); err != nil {
		t.Fatal(err)
	}
	first, second := <-received, <-received
	if first.Inputs[0].Audio == nil || *first.Inputs[0].Audio != *reference || first.Inputs[0].Video != nil || second.Inputs[0].Text != text {
		t.Fatalf("audio reference or original Chinese query changed: %+v %+v", first, second)
	}
	for _, input := range []EmbedInput{
		{Audio: &MediaRef{URI: reference.URI, StartMs: -1, EndMs: 1}},
		{Audio: &MediaRef{URI: reference.URI, StartMs: 100, EndMs: 100}},
		{Audio: &MediaRef{URI: reference.URI, StartMs: 0, EndMs: 10001}},
		{Audio: &MediaRef{URI: "https://example.test/video.mp4", EndMs: 1000}},
		{Audio: reference, Text: text},
		{Audio: reference, Video: reference},
	} {
		if err := validateEmbedRequest(EmbedRequest{Role: "document", Inputs: []EmbedInput{input}}, info); err == nil {
			t.Fatalf("invalid audio input accepted: %+v", input)
		}
	}
	if err := validateEmbedRequest(EmbedRequest{Role: "query", Inputs: []EmbedInput{{Audio: reference}}}, info); err == nil {
		t.Fatal("audio query accepted; search remains text only")
	}
}
