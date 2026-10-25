package library

import (
	"context"
	"slices"

	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/library/librarydb"
	"github.com/ozine/videohelper/internal/workspace"
)

type Scope struct {
	ProjectID *int64  `json:"project_id,omitempty"`
	VideoIDs  []int64 `json:"video_ids,omitempty"`
}

func (s *Service) ResolveScope(ctx context.Context, p workspace.Principal, sc Scope) ([]int64, error) {
	if sc.ProjectID != nil {
		if sc.VideoIDs != nil {
			return nil, apperr.New(apperr.Invalid, "invalid_scope", "项目和视频范围不能同时指定")
		}
		if _, err := librarydb.New(s.pool).Project(ctx, librarydb.ProjectParams{ID: *sc.ProjectID, WorkspaceID: p.WorkspaceID}); err != nil {
			return nil, rowError(err)
		}
		return librarydb.New(s.pool).ProjectVideoIDs(ctx, librarydb.ProjectVideoIDsParams{ProjectID: *sc.ProjectID, WorkspaceID: p.WorkspaceID})
	}
	if sc.VideoIDs == nil {
		return nil, nil
	}
	if len(sc.VideoIDs) == 0 {
		return []int64{}, nil
	}
	ids := slices.Clone(sc.VideoIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	matched, err := librarydb.New(s.pool).ScopedVideoIDs(ctx, librarydb.ScopedVideoIDsParams{WorkspaceID: p.WorkspaceID, Column2: ids})
	if err != nil {
		return nil, err
	}
	if len(matched) != len(ids) {
		return nil, notFound()
	}
	return matched, nil
}

func (s *Service) Durations(ctx context.Context, workspaceID int64, ids []int64) (map[int64]int, error) {
	rows, err := librarydb.New(s.pool).Durations(ctx, librarydb.DurationsParams{WorkspaceID: workspaceID, Column2: ids})
	if err != nil {
		return nil, err
	}
	durations := make(map[int64]int, len(rows))
	for _, row := range rows {
		durations[row.ID] = int(row.DurationMs.Int32)
	}
	return durations, nil
}
