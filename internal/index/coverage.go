package index

import (
	"context"

	"github.com/ozine/videohelper/internal/platform/db"
)

type CoverageCounts struct {
	EligibleVideos int
	IndexedVideos  int
}

// SpaceCoverage counts whole-video completion, including speech stages with no sentences.
// Nil videoIDs selects the workspace; a nonnil empty slice selects nothing.
func SpaceCoverage(ctx context.Context, q db.Querier, workspaceID int64, videoIDs []int64, channel, seg, spaceID, fingerprint string) (CoverageCounts, error) {
	var counts CoverageCounts
	if videoIDs != nil && len(videoIDs) == 0 {
		return counts, nil
	}
	err := q.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE EXISTS (
SELECT 1 FROM stage_run s
WHERE s.video_id=v.id AND s.stage='embed:' || $3 || ':' || $5 AND s.status='done'
AND s.output #>> '{plan,options,channel}'=$3
AND s.output #>> '{plan,options,seg}'=$4
AND s.output #>> '{plan,options,space,ID}'=$5
AND s.output #>> '{plan,options,space,Fingerprint}'=$6))
FROM video v
WHERE v.workspace_id=$1 AND ($2::bigint[] IS NULL OR v.id=ANY($2))
AND v.status='ready' AND v.deleted_at IS NULL
AND (($3='visual' AND v.has_video) OR ($3 IN ('speech','audio') AND v.has_audio))`,
		workspaceID, videoIDs, channel, seg, spaceID, fingerprint).Scan(&counts.EligibleVideos, &counts.IndexedVideos)
	return counts, err
}
