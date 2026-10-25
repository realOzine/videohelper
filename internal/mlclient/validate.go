package mlclient

import (
	"math"
	"regexp"
	"slices"
)

var (
	identifier  = regexp.MustCompile(`^[a-z0-9@._-]+$`)
	fingerprint = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	s3URI       = regexp.MustCompile(`^s3://[a-z0-9][a-z0-9.-]*/[^?#\r\n]+$`)
)

func validateSpaces(response SpacesResponse) error {
	if response.Spaces == nil || response.Transcribers == nil {
		return contract("missing discovery arrays")
	}
	seen := make(map[string]bool)
	for _, info := range response.Spaces {
		if !identifier.MatchString(info.ID) || seen[info.ID] || info.Dim <= 0 || info.MaxBatch <= 0 ||
			!slices.Contains([]string{"cosine", "ip", "l2"}, info.Metric) || !fingerprint.MatchString(info.Fingerprint) ||
			!validInputTypes(info.DocInputs) || !validInputTypes(info.QueryInputs) || (info.MaxSpanMs != nil && *info.MaxSpanMs <= 0) {
			return contract("invalid or duplicate space metadata")
		}
		seen[info.ID] = true
	}
	clear(seen)
	for _, info := range response.Transcribers {
		if !identifier.MatchString(info.ID) || seen[info.ID] || info.MaxWindowMs <= 0 || !fingerprint.MatchString(info.Fingerprint) || len(info.Languages) == 0 || slices.Contains(info.Languages, "") {
			return contract("invalid or duplicate transcriber metadata")
		}
		seen[info.ID] = true
	}
	return nil
}

func validInputTypes(types []string) bool {
	if len(types) == 0 {
		return false
	}
	for _, value := range types {
		if !slices.Contains([]string{"text", "video", "image", "audio"}, value) {
			return false
		}
	}
	return true
}

func validMedia(uri string, start, end int) bool {
	return s3URI.MatchString(uri) && start >= 0 && end > start
}

func validateEmbedRequest(req EmbedRequest, info SpaceInfo) error {
	if (req.Role != "query" && req.Role != "document") || len(req.Inputs) == 0 || len(req.Inputs) > info.MaxBatch {
		return invalid("invalid embedding role or batch size")
	}
	allowed := info.DocInputs
	if req.Role == "query" {
		allowed = info.QueryInputs
	}
	for _, input := range req.Inputs {
		count, kind := 0, ""
		if input.Text != "" {
			count, kind = count+1, "text"
		}
		if input.Video != nil {
			count, kind = count+1, "video"
			video := input.Video
			if !validMedia(video.URI, video.StartMs, video.EndMs) || (info.MaxSpanMs != nil && video.EndMs-video.StartMs > *info.MaxSpanMs) {
				return invalid("invalid video reference or window")
			}
		}
		if input.Audio != nil {
			count, kind = count+1, "audio"
			audio := input.Audio
			if !validMedia(audio.URI, audio.StartMs, audio.EndMs) || (info.MaxSpanMs != nil && audio.EndMs-audio.StartMs > *info.MaxSpanMs) {
				return invalid("invalid audio reference or window")
			}
		}
		if input.Image != nil {
			count, kind = count+1, "image"
			if !s3URI.MatchString(input.Image.URI) {
				return invalid("invalid image reference")
			}
		}
		if count != 1 || !slices.Contains(allowed, kind) || (req.Role == "query" && kind != "text") {
			return invalid("unsupported or ambiguous embedding input")
		}
	}
	return nil
}

func validateVectors(req EmbedRequest, response EmbedResponse, info SpaceInfo) error {
	if response.Space != info.ID || response.Fingerprint != info.Fingerprint || response.Dim != info.Dim || len(response.Vectors) != len(req.Inputs) {
		return contract("embedding identity or vector count changed")
	}
	for _, vector := range response.Vectors {
		if len(vector) != info.Dim {
			return contract("embedding vector dimension changed")
		}
		var squaredNorm float64
		for _, value := range vector {
			v := float64(value)
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return contract("nonfinite embedding vector")
			}
			squaredNorm += v * v
		}
		if info.Metric == "cosine" && math.Abs(math.Sqrt(squaredNorm)-1) > 1e-3 {
			return contract("cosine vector is not normalized")
		}
	}
	return nil
}

// ValidateTranscript also validates persisted chunks before checkpoint replay.
func ValidateTranscript(req TranscribeRequest, response TranscribeResponse, info TranscriberInfo) error {
	if response.Model != info.ID || response.Fingerprint != info.Fingerprint || response.DurationMs < 0 || response.Sentences == nil || !slices.Contains(info.Languages, response.Language) {
		return contract("invalid transcription identity or metadata")
	}
	end, previousSentence, previousWord := min(req.EndMs, response.DurationMs), req.StartMs, req.StartMs
	for _, sentence := range response.Sentences {
		if sentence.StartMs < previousSentence || sentence.EndMs <= sentence.StartMs || sentence.EndMs > end || sentence.Words == nil {
			return contract("invalid or out-of-order sentence timestamps")
		}
		for _, word := range sentence.Words {
			if word.S < sentence.StartMs || word.S < previousWord || word.E <= word.S || word.E > sentence.EndMs {
				return contract("word timestamps outside sentence or out of order")
			}
			previousWord = word.S
		}
		previousSentence = sentence.StartMs
	}
	return nil
}
