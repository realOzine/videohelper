-- name: LockSpeechVideo :one
SELECT id FROM video
WHERE id = $1 AND workspace_id = $2 AND deleted_at IS NULL
FOR UPDATE;

-- name: DeleteSpeechText :exec
DELETE FROM text_span WHERE video_id = $1 AND workspace_id = $2 AND kind = 'speech';

-- name: DeleteSpeechVectors :exec
DELETE FROM embedding WHERE video_id = $1 AND workspace_id = $2 AND channel = 'speech';

-- name: InsertSpeech :exec
INSERT INTO text_span (video_id, workspace_id, kind, start_ms, end_ms, text, words, model)
VALUES ($1, $2, 'speech', $3, $4, $5, $6, $7);

-- name: TextRange :many
SELECT t.* FROM text_span t JOIN video v ON v.id = t.video_id AND v.workspace_id = t.workspace_id
WHERE t.workspace_id = $1 AND t.video_id = $2
  AND t.start_ms < sqlc.arg(range_end_ms) AND t.end_ms > sqlc.arg(range_start_ms)
  AND v.deleted_at IS NULL
ORDER BY t.start_ms, t.kind;

-- name: SpeechSentences :many
SELECT t.* FROM text_span t JOIN video v ON v.id = t.video_id AND v.workspace_id = t.workspace_id
WHERE t.video_id = $1 AND t.kind = 'speech' AND v.deleted_at IS NULL
ORDER BY t.start_ms;

-- name: DeleteVideoChannel :exec
DELETE FROM embedding WHERE video_id = $1 AND channel = $2;

-- name: DeleteVideo :exec
WITH vectors AS (DELETE FROM embedding e WHERE e.video_id = $1)
DELETE FROM text_span t WHERE t.video_id = $1;

-- name: Coverage :one
SELECT count(DISTINCT e.video_id) FROM embedding e JOIN video v ON v.id = e.video_id AND v.workspace_id = e.workspace_id
WHERE e.channel = $1 AND e.space_id = $2 AND v.status = 'ready' AND v.deleted_at IS NULL;

-- name: DeleteSpaceBatch :execrows
DELETE FROM embedding e WHERE e.ctid IN (
  SELECT b.ctid FROM embedding b WHERE b.space_id = $1 LIMIT $2
);

-- name: DisableSpace :exec
UPDATE embedding_space SET disabled = true WHERE id = $1;
