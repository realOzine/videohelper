-- name: Video :one
SELECT * FROM video WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL;

-- name: VideoForWorker :one
SELECT * FROM video WHERE id = $1 AND deleted_at IS NULL FOR UPDATE;

-- name: ListVideos :many
SELECT v.* FROM video v
WHERE v.workspace_id = @workspace_id AND v.deleted_at IS NULL
  AND (@status::text = '' OR v.status = @status)
  AND (sqlc.narg(project_id)::bigint IS NULL OR EXISTS (
    SELECT 1 FROM project_video pv WHERE pv.project_id = sqlc.narg(project_id) AND pv.video_id = v.id
  ))
  AND (sqlc.narg(before_time)::timestamptz IS NULL OR (v.created_at, v.id) < (sqlc.narg(before_time), @before_id::bigint))
ORDER BY v.created_at DESC, v.id DESC LIMIT @page_limit;

-- name: Project :one
SELECT * FROM project WHERE id = $1 AND workspace_id = $2;

-- name: CreateProject :one
INSERT INTO project (workspace_id, name) VALUES ($1, $2) RETURNING *;

-- name: ListProjects :many
SELECT * FROM project WHERE workspace_id = $1 ORDER BY created_at DESC, id DESC;

-- name: DeleteProject :execrows
DELETE FROM project WHERE id = $1 AND workspace_id = $2;

-- name: AddVideoToProject :one
WITH allowed AS (
  SELECT p.id AS project_id, v.id AS video_id FROM project p, video v
  WHERE p.id = @project_id AND v.id = @video_id
    AND p.workspace_id = @workspace_id AND v.workspace_id = @workspace_id AND v.deleted_at IS NULL
), inserted AS (
  INSERT INTO project_video (project_id, video_id) SELECT * FROM allowed
  ON CONFLICT DO NOTHING
)
SELECT EXISTS (SELECT 1 FROM allowed);

-- name: RemoveVideoFromProject :one
WITH allowed AS (
  SELECT p.id AS project_id, v.id AS video_id FROM project p, video v
  WHERE p.id = @project_id AND v.id = @video_id
    AND p.workspace_id = @workspace_id AND v.workspace_id = @workspace_id AND v.deleted_at IS NULL
), removed AS (
  DELETE FROM project_video pv USING allowed a WHERE pv.project_id = a.project_id AND pv.video_id = a.video_id
)
SELECT EXISTS (SELECT 1 FROM allowed);

-- name: ProjectVideoIDs :many
SELECT v.id FROM video v JOIN project_video pv ON pv.video_id = v.id
WHERE pv.project_id = $1 AND v.workspace_id = $2 AND v.deleted_at IS NULL ORDER BY v.id;

-- name: ScopedVideoIDs :many
SELECT id FROM video WHERE workspace_id = $1 AND id = ANY($2::bigint[]) AND deleted_at IS NULL ORDER BY id;

-- name: Durations :many
SELECT id, duration_ms FROM video
WHERE workspace_id = $1 AND id = ANY($2::bigint[]) AND deleted_at IS NULL AND duration_ms IS NOT NULL;

-- name: UpdateSource :execrows
UPDATE video SET source_key = $2, status = CASE WHEN status = 'importing' THEN 'indexing' ELSE status END, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateProbe :execrows
UPDATE video SET duration_ms = $2, has_video = $3, has_audio = $4, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdatePreparedMedia :execrows
UPDATE video SET normalized_video_key = $2, thumbs = $3, updated_at = now()
WHERE id = $1 AND deleted_at IS NULL;

-- name: SetStatus :execrows
UPDATE video SET status = $2, error = $3, updated_at = now() WHERE id = $1 AND deleted_at IS NULL;

-- name: CreateUploadVideo :one
INSERT INTO video (workspace_id, title, source_kind, status) VALUES ($1, $2, 'upload', 'uploading') RETURNING *;

-- name: SaveUpload :exec
INSERT INTO video_upload (video_id, object_key, upload_id, size_bytes, part_size) VALUES ($1, $2, $3, $4, $5);

-- name: Upload :one
SELECT u.* FROM video_upload u JOIN video v ON v.id = u.video_id
WHERE u.video_id = $1 AND v.workspace_id = $2 AND v.deleted_at IS NULL;

-- name: RemoveUnstartedUpload :exec
DELETE FROM video WHERE id = $1 AND status = 'uploading' AND source_key IS NULL AND deleted_at IS NULL;

-- name: VideoIncludingDeleted :one
SELECT * FROM video WHERE id = $1 AND workspace_id = $2 FOR UPDATE;

-- name: SoftDelete :exec
UPDATE video SET deleted_at = now(), updated_at = now() WHERE id = $1 AND workspace_id = $2;

-- name: RestoreVideo :exec
UPDATE video SET deleted_at = NULL, status = $3, error = NULL, updated_at = now() WHERE id = $1 AND workspace_id = $2;
