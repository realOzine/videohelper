-- name: Stages :many
SELECT * FROM stage_run WHERE video_id = $1 ORDER BY stage, version FOR UPDATE;

-- name: ReadStage :one
SELECT s.* FROM stage_run s JOIN video v ON v.id = s.video_id
WHERE s.video_id = $1 AND s.stage = $2 AND s.version = $3
  AND v.workspace_id = $4 AND v.deleted_at IS NULL AND v.status IN ('indexing', 'ready');

-- name: LockStage :one
SELECT * FROM stage_run WHERE video_id = $1 AND stage = $2 AND version = $3 FOR UPDATE;

-- name: InsertStage :exec
INSERT INTO stage_run (video_id, stage, version, status, required, output)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ClaimStage :one
UPDATE stage_run SET status = 'running', attempt = attempt + 1, updated_at = now()
WHERE video_id = $1 AND stage = $2 AND version = $3 AND status = 'queued'
RETURNING attempt;

-- name: UpdateStage :exec
UPDATE stage_run SET status = $4, error = $5, output = $6, updated_at = now()
WHERE video_id = $1 AND stage = $2 AND version = $3;

-- name: RetryStage :exec
UPDATE stage_run SET version = sqlc.arg(new_version), status = sqlc.arg(new_status), error = NULL,
  attempt = 0, updated_at = now()
WHERE video_id = $1 AND stage = $2 AND version = $3;

-- name: DeleteStages :exec
DELETE FROM stage_run WHERE video_id = $1;

-- name: ReconcileStages :many
SELECT s.*, v.workspace_id FROM stage_run s JOIN video v ON v.id = s.video_id
WHERE ((s.status = 'queued' AND s.updated_at < sqlc.arg(before_time)::timestamptz)
  OR (s.status = 'running' AND s.updated_at + interval '10 minutes'
      + interval '1 second' * ((s.output->'plan'->>'timeout')::double precision / 1000000000)
      < sqlc.arg(now_time)::timestamptz))
  AND v.deleted_at IS NULL AND v.status IN ('indexing', 'ready')
ORDER BY s.updated_at LIMIT 100;

-- name: UnstartedUploads :many
SELECT v.id FROM video v
WHERE v.status = 'uploading' AND v.source_kind = 'upload' AND v.source_key IS NOT NULL
  AND v.deleted_at IS NULL AND NOT EXISTS (SELECT 1 FROM stage_run s WHERE s.video_id = v.id)
ORDER BY v.id LIMIT 100;

-- name: ListStages :many
SELECT s.* FROM stage_run s JOIN video v ON v.id = s.video_id
WHERE s.video_id = $1 AND v.workspace_id = $2 AND v.deleted_at IS NULL
ORDER BY s.stage;

-- name: BackfillVideos :many
WITH targets AS (
  SELECT unnest(@stages::text[]) AS stage, unnest(@channels::text[]) AS channel, unnest(@deps::text[]) AS dep
)
SELECT v.id FROM video v
WHERE v.status='ready' AND v.deleted_at IS NULL
  AND v.normalized_video_key IS NOT NULL AND v.normalized_video_key<>'' AND v.duration_ms>0
  AND EXISTS (
    SELECT 1 FROM targets t
    WHERE ((t.channel='visual' AND v.has_video) OR (t.channel IN ('audio','speech') AND v.has_audio))
      AND NOT EXISTS (SELECT 1 FROM stage_run s WHERE s.video_id=v.id AND s.stage=t.stage)
      AND EXISTS (SELECT 1 FROM stage_run s WHERE s.video_id=v.id AND s.stage=t.dep AND s.status='done')
  )
ORDER BY v.id LIMIT 100;

-- name: BackfillSpace :one
SELECT id FROM embedding_space
WHERE id=$1 AND NOT disabled AND fingerprint=$2 AND dim=$3 AND metric=$4 FOR SHARE;
