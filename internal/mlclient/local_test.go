package mlclient

import "testing"

func TestLocalMediaReferences(t *testing.T) {
	for _, uri := range []string{"local://media/source.mp4", "local://media/video-1/original.mkv"} {
		if !validMedia(uri, 0, 1000) {
			t.Fatalf("rejected valid local reference %q", uri)
		}
		info := testMetadata().Spaces[0]
		if err := validateEmbedRequest(EmbedRequest{Space: info.ID, Role: "document", Inputs: []EmbedInput{{Image: &ImageRef{URI: uri}}}}, info); err != nil {
			t.Fatalf("local image: %v", err)
		}
	}
	for _, key := range []string{"../secret", "/etc/passwd", "a/../b", "a/%2e%2e/b", `a\b`, "a?x", "a#x", "a\n", ".", "a//b"} {
		if validMedia("local://media/"+key, 0, 1000) {
			t.Fatalf("accepted unsafe key %q", key)
		}
	}
}

func TestExternalMediaProtocolsRejected(t *testing.T) {
	for _, uri := range []string{"s3://bucket/key", "https://example.org/video", "file:///etc/passwd"} {
		if validURI(uri) {
			t.Fatalf("accepted external protocol %q", uri)
		}
	}
}
