package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/library"
	"github.com/ozine/videohelper/internal/platform/blob"
	"github.com/ozine/videohelper/internal/workspace"
)

func parseID(raw string) (int64, error) {
	if raw == "" || raw[0] < '1' || raw[0] > '9' {
		return 0, apperr.New(apperr.Invalid, "invalid_argument", "ID 必须为正整数字符串")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, apperr.New(apperr.Invalid, "invalid_argument", "ID 必须为正整数字符串")
	}
	return id, nil
}

func optionalID(raw *string) (*int64, error) {
	if raw == nil {
		return nil, nil
	}
	id, err := parseID(*raw)
	return &id, err
}

func principal(r *http.Request) workspace.Principal {
	p, _ := workspace.PrincipalFrom(r.Context())
	return p
}

func publicVideo(v library.Video) library.Video {
	v.SourceURL = ""
	v.CreatedAt, v.UpdatedAt = v.CreatedAt.UTC(), v.UpdatedAt.UTC()
	if v.DeletedAt != nil {
		utc := v.DeletedAt.UTC()
		v.DeletedAt = &utc
	}
	return v
}

func (a api) me(r *http.Request) (int, any, error) {
	p := principal(r)
	return 200, map[string]any{"user_id": strconv.FormatInt(p.UserID, 10), "workspace_id": strconv.FormatInt(p.WorkspaceID, 10), "role": p.Role}, nil
}

type projectResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	VideoCount int       `json:"video_count"`
	CreatedAt  time.Time `json:"created_at"`
}

func (a api) createProject(r *http.Request) (int, any, error) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decode(r, &body, false); err != nil {
		return 0, nil, err
	}
	project, err := a.Library.CreateProject(r.Context(), principal(r), body.Name)
	if err != nil {
		return 0, nil, err
	}
	return 201, map[string]any{"project": projectResponse{ID: strconv.FormatInt(project.ID, 10), Name: project.Name, CreatedAt: project.CreatedAt.UTC()}}, nil
}

func (a api) projects(r *http.Request) (int, any, error) {
	p := principal(r)
	projects, err := a.Library.ListProjects(r.Context(), p)
	if err != nil {
		return 0, nil, err
	}
	public := make([]projectResponse, 0, len(projects))
	// shortcut: counts use one scoped lookup per project, batch counts when project listings become large.
	for _, project := range projects {
		ids, err := a.Library.ResolveScope(r.Context(), p, library.Scope{ProjectID: &project.ID})
		if err != nil {
			return 0, nil, err
		}
		public = append(public, projectResponse{ID: strconv.FormatInt(project.ID, 10), Name: project.Name, VideoCount: len(ids), CreatedAt: project.CreatedAt.UTC()})
	}
	return 200, map[string]any{"projects": public}, nil
}

func (a api) deleteProject(r *http.Request) (int, any, error) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	if err := decode(r, new(struct{}), true); err != nil {
		return 0, nil, err
	}
	return 204, nil, a.Library.DeleteProject(r.Context(), principal(r), id)
}

func (a api) projectVideo(r *http.Request, add bool) (int, any, error) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	video, err := parseID(r.PathValue("video_id"))
	if err != nil {
		return 0, nil, err
	}
	if err := decode(r, new(struct{}), true); err != nil {
		return 0, nil, err
	}
	if add {
		err = a.Library.AddVideo(r.Context(), principal(r), id, video)
	} else {
		err = a.Library.RemoveVideo(r.Context(), principal(r), id, video)
	}
	return 204, nil, err
}

func (a api) addVideo(r *http.Request) (int, any, error)    { return a.projectVideo(r, true) }
func (a api) removeVideo(r *http.Request) (int, any, error) { return a.projectVideo(r, false) }

func (a api) startUpload(r *http.Request) (int, any, error) {
	var body struct {
		Filename    string  `json:"filename"`
		Size        int64   `json:"size_bytes"`
		ContentType string  `json:"content_type"`
		ProjectID   *string `json:"project_id"`
	}
	if err := decode(r, &body, false); err != nil {
		return 0, nil, err
	}
	id, err := optionalID(body.ProjectID)
	if err != nil {
		return 0, nil, err
	}
	video, plan, err := a.Library.StartUpload(r.Context(), principal(r), body.Filename, body.Size, body.ContentType, id)
	if err != nil {
		return 0, nil, err
	}
	plan.ExpiresAt = plan.ExpiresAt.UTC()
	return 201, map[string]any{"video": publicVideo(video), "upload": plan}, nil
}

func (a api) completeUpload(r *http.Request) (int, any, error) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	var body struct {
		UploadID string      `json:"upload_id"`
		Parts    []blob.Part `json:"parts"`
	}
	if err := decode(r, &body, false); err != nil {
		return 0, nil, err
	}
	video, err := a.Library.CompleteUpload(r.Context(), principal(r), id, body.UploadID, body.Parts)
	if err != nil {
		return 0, nil, err
	}
	return 202, map[string]any{"video": publicVideo(video)}, nil
}

func (a api) videos(r *http.Request) (int, any, error) {
	params := r.URL.Query()
	filter := library.ListFilter{Status: params.Get("status"), Cursor: params.Get("cursor")}
	var err error
	if raw := params.Get("project_id"); raw != "" {
		id, err := parseID(raw)
		if err != nil {
			return 0, nil, err
		}
		filter.ProjectID = &id
	}
	if raw := params.Get("limit"); raw != "" {
		filter.Limit, err = strconv.Atoi(raw)
		if err != nil {
			return 0, nil, apperr.New(apperr.Invalid, "invalid_argument", "返回数量不合法")
		}
	}
	videos, next, err := a.Library.List(r.Context(), principal(r), filter)
	if err != nil {
		return 0, nil, err
	}
	for i := range videos {
		videos[i] = publicVideo(videos[i])
	}
	var cursor *string
	if next != "" {
		cursor = &next
	}
	return 200, map[string]any{"videos": videos, "next_cursor": cursor}, nil
}

func (a api) video(r *http.Request) (int, any, error) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	video, err := a.Library.Get(r.Context(), principal(r), id)
	if err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"video": publicVideo(video)}, nil
}

func (a api) deleteVideo(r *http.Request) (int, any, error) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	if err := decode(r, new(struct{}), true); err != nil {
		return 0, nil, err
	}
	return 202, nil, a.Library.Delete(r.Context(), principal(r), id)
}

func (a api) restoreVideo(r *http.Request) (int, any, error) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	if err := decode(r, new(struct{}), true); err != nil {
		return 0, nil, err
	}
	video, err := a.Library.Restore(r.Context(), principal(r), id)
	if err != nil {
		return 0, nil, err
	}
	return 202, map[string]any{"video": publicVideo(video)}, nil
}

func (a api) retryVideo(r *http.Request) (int, any, error) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	if err := decode(r, new(struct{}), true); err != nil {
		return 0, nil, err
	}
	if a.Pipeline == nil {
		return 0, nil, apperr.New(apperr.Unavailable, "unavailable", "索引服务尚未就绪")
	}
	return 202, nil, a.Pipeline.Retry(r.Context(), principal(r).WorkspaceID, id)
}

func (a api) playback(r *http.Request) (int, any, error) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		return 0, nil, err
	}
	url, expires, err := a.Library.PlaybackURL(r.Context(), principal(r), id)
	if err != nil {
		return 0, nil, err
	}
	return 200, map[string]any{"url": url, "expires_at": expires.UTC()}, nil
}
