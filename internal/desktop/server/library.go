package server

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/desktop/processor"
	"github.com/ozine/videohelper/internal/desktop/store"
)

func (s *Server) projects(r *http.Request) (any, error) {
	rows, err := s.Store.ListProjects(r.Context())
	return map[string]any{"projects": rows}, err
}

func (s *Server) createProject(r *http.Request) (any, error) {
	var b struct {
		Name string `json:"name"`
	}
	if err := decode(r, &b); err != nil {
		return nil, err
	}
	p, err := s.Store.CreateProject(r.Context(), b.Name)
	return map[string]any{"project": p}, err
}

func (s *Server) deleteProject(r *http.Request) (any, error) {
	n, err := id(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	return nil, s.Store.DeleteProject(r.Context(), n)
}

func (s *Server) addProjectVideo(r *http.Request) (any, error) {
	p, err := id(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	v, err := id(r.PathValue("video_id"))
	if err != nil {
		return nil, err
	}
	return nil, s.Store.AddProjectVideo(r.Context(), p, v)
}

func (s *Server) removeProjectVideo(r *http.Request) (any, error) {
	p, err := id(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	v, err := id(r.PathValue("video_id"))
	if err != nil {
		return nil, err
	}
	return nil, s.Store.RemoveProjectVideo(r.Context(), p, v)
}

func (s *Server) videos(r *http.Request) (any, error) {
	q := r.URL.Query()
	project, err := optionalID(q.Get("project_id"))
	if err != nil {
		return nil, err
	}
	cursor, err := optionalID(q.Get("cursor"))
	if err != nil {
		return nil, err
	}
	limit := 50
	if q.Has("limit") {
		limit, err = strconv.Atoi(q.Get("limit"))
		if err != nil || limit < 1 || limit > 100 {
			return nil, store.ErrInvalid
		}
	}
	status := q.Get("status")
	f := store.VideoFilter{ProjectID: project, Status: status, BeforeID: cursor, Limit: limit + 1}
	if status == "deleted" {
		f.Status = ""
		f.Deleted = true
	}
	rows, err := s.Store.ListVideos(r.Context(), f)
	if err != nil {
		return nil, err
	}
	var next *string
	if len(rows) > limit {
		rows = rows[:limit]
		n := strconv.FormatInt(rows[len(rows)-1].ID, 10)
		next = &n
	}
	out := make([]videoDTO, 0, len(rows))
	for _, v := range rows {
		out = append(out, s.public(v))
	}
	return map[string]any{"videos": out, "next_cursor": next}, nil
}

func (s *Server) video(r *http.Request) (any, error) {
	n, err := id(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	v, err := s.Store.Video(r.Context(), n)
	if err != nil {
		return nil, err
	}
	stages := []processor.StageStatus{}
	if p, err := s.engine(); err == nil {
		stages, err = p.Stages(r.Context(), n)
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{"video": s.public(v), "stages": stages}, nil
}

func (s *Server) deleteVideo(r *http.Request) (any, error) {
	n, err := id(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	if p, err := s.engine(); err == nil {
		if err := p.Cancel(r.Context(), n); err != nil {
			return nil, err
		}
	}
	return nil, s.Store.DeleteVideo(r.Context(), n)
}

func (s *Server) restoreVideo(r *http.Request) (any, error) {
	n, err := id(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	v, err := s.Store.RestoreVideo(r.Context(), n)
	return map[string]any{"video": s.public(v)}, err
}

func (s *Server) retryVideo(r *http.Request) (any, error) {
	n, err := id(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	p, err := s.engine()
	if err != nil {
		return nil, err
	}
	j, err := p.Retry(r.Context(), n)
	return map[string]any{"job": j}, err
}

func (s *Server) cancelVideo(r *http.Request) (any, error) {
	n, err := id(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	p, err := s.engine()
	if err != nil {
		// Imports are durable while models prepare. They must remain cancellable
		// before a processor exists; no in-flight model process exists here.
		j, err := s.Store.LatestJob(r.Context(), n)
		if errors.Is(err, store.ErrNotFound) || err == nil && j.Status != "queued" && j.Status != "running" {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		err = s.Store.CancelJob(r.Context(), j.ID)
		if errors.Is(err, store.ErrConflict) {
			err = nil // The job may have completed while cancellation was requested.
		}
		return nil, err
	}
	return nil, p.Cancel(r.Context(), n)
}

func (s *Server) playback(r *http.Request) (any, error) {
	n, err := id(r.PathValue("id"))
	if err != nil {
		return nil, err
	}
	v, err := s.Store.Video(r.Context(), n)
	if err != nil {
		return nil, err
	}
	if v.DeletedAt != nil {
		return nil, store.ErrNotFound
	}
	if v.NormalizedVideoKey == "" {
		return nil, apperr.New(apperr.Unavailable, "media_not_ready", "Video is still preparing")
	}
	return map[string]any{"url": s.mediaURL(v, v.NormalizedVideoKey)}, nil
}

func (s *Server) importFile(r *http.Request) (any, error) {
	var b struct {
		Path      string `json:"path"`
		ProjectID string `json:"project_id"`
	}
	if err := decode(r, &b); err != nil {
		return nil, err
	}
	project, err := optionalID(b.ProjectID)
	if err != nil {
		return nil, err
	}
	var v store.Video
	if p, ready := s.engine(); ready == nil {
		v, err = p.ImportFile(r.Context(), b.Path, project)
	} else {
		v, err = s.Store.ImportFile(r.Context(), b.Path, project)
	}
	return map[string]any{"video": s.public(v)}, err
}

func (s *Server) importURL(r *http.Request) (any, error) {
	var b struct {
		URL       string `json:"url"`
		ProjectID string `json:"project_id"`
	}
	if err := decode(r, &b); err != nil {
		return nil, err
	}
	project, err := optionalID(b.ProjectID)
	if err != nil {
		return nil, err
	}
	var v store.Video
	if p, ready := s.engine(); ready == nil {
		v, err = p.ImportURL(r.Context(), b.URL, project)
	} else {
		v, err = s.Store.ImportURL(r.Context(), b.URL, project)
	}
	return map[string]any{"video": s.public(v)}, err
}
