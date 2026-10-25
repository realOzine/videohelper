package blob

import (
	"fmt"
	"strings"
)

func VideoPrefix(workspaceID, videoID int64) string {
	return fmt.Sprintf("workspaces/%d/videos/%d/", workspaceID, videoID)
}

func SourceKey(workspaceID, videoID int64, ext string) string {
	return VideoPrefix(workspaceID, videoID) + "source." + strings.TrimPrefix(ext, ".")
}

func NormalizedVideoKey(workspaceID, videoID int64) string {
	return VideoPrefix(workspaceID, videoID) + "normalized_video.mp4"
}

func SpriteKey(workspaceID, videoID int64, n int) string {
	return fmt.Sprintf("%ssprites/%d.jpg", VideoPrefix(workspaceID, videoID), n)
}

func RawKey(workspaceID, videoID int64, name string) string {
	return VideoPrefix(workspaceID, videoID) + "raw/" + name
}

// RunPrefix keeps late or retried processing attempts from overwriting published artifacts.
func RunPrefix(workspaceID, videoID int64, generation string, attempt int) string {
	return fmt.Sprintf("%sruns/%s/%d/", VideoPrefix(workspaceID, videoID), generation, attempt)
}

func RunNormalizedVideoKey(workspaceID, videoID int64, generation string, attempt int) string {
	return RunPrefix(workspaceID, videoID, generation, attempt) + "normalized_video.mp4"
}

func RunSpriteKey(workspaceID, videoID int64, generation string, attempt, n int) string {
	return fmt.Sprintf("%ssprites/%d.jpg", RunPrefix(workspaceID, videoID, generation, attempt), n)
}

func RunRawKey(workspaceID, videoID int64, generation string, attempt int, name string) string {
	return RunPrefix(workspaceID, videoID, generation, attempt) + "raw/" + name
}

// Sprites belong to the same immutable run as the video key stored in the database.
func SpriteKeyForVideo(normalizedVideoKey string, n int) string {
	prefix, _, _ := strings.Cut(normalizedVideoKey, "normalized_video.mp4")
	return fmt.Sprintf("%ssprites/%d.jpg", prefix, n)
}
