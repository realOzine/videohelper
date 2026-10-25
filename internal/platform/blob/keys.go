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
