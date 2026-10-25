-- name: RecordIndexed :exec
INSERT INTO usage_event (workspace_id,kind,amount,ref) VALUES ($1,'indexed_ms',$2,$3);
