package library

import (
	"context"
	"strings"

	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/library/librarydb"
	"github.com/ozine/videohelper/internal/workspace"
)

func (s *Service) CreateProject(ctx context.Context, p workspace.Principal, name string) (Project, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Project{}, apperr.New(apperr.Invalid, "invalid_project", "项目名称不能为空")
	}
	row, err := librarydb.New(s.pool).CreateProject(ctx, librarydb.CreateProjectParams{WorkspaceID: p.WorkspaceID, Name: name})
	return projectFromRow(row), err
}

func (s *Service) ListProjects(ctx context.Context, p workspace.Principal) ([]Project, error) {
	rows, err := librarydb.New(s.pool).ListProjects(ctx, p.WorkspaceID)
	if err != nil {
		return nil, err
	}
	projects := make([]Project, 0, len(rows))
	for _, row := range rows {
		projects = append(projects, projectFromRow(row))
	}
	return projects, nil
}

func (s *Service) DeleteProject(ctx context.Context, p workspace.Principal, id int64) error {
	return affected(librarydb.New(s.pool).DeleteProject(ctx, librarydb.DeleteProjectParams{ID: id, WorkspaceID: p.WorkspaceID}))
}

func (s *Service) AddVideo(ctx context.Context, p workspace.Principal, projectID, videoID int64) error {
	allowed, err := librarydb.New(s.pool).AddVideoToProject(ctx, librarydb.AddVideoToProjectParams{WorkspaceID: p.WorkspaceID, ProjectID: projectID, VideoID: videoID})
	if err != nil {
		return err
	}
	if !allowed {
		return notFound()
	}
	return nil
}

func (s *Service) RemoveVideo(ctx context.Context, p workspace.Principal, projectID, videoID int64) error {
	allowed, err := librarydb.New(s.pool).RemoveVideoFromProject(ctx, librarydb.RemoveVideoFromProjectParams{WorkspaceID: p.WorkspaceID, ProjectID: projectID, VideoID: videoID})
	if err != nil {
		return err
	}
	if !allowed {
		return notFound()
	}
	return nil
}
