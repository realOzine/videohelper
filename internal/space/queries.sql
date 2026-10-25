-- name: RegisterSpace :one
INSERT INTO embedding_space (id,dim,metric,fingerprint,doc_inputs,query_inputs,max_span_ms)
VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (id) DO UPDATE SET disabled = embedding_space.disabled
 OR embedding_space.dim <> EXCLUDED.dim
 OR embedding_space.metric <> EXCLUDED.metric
 OR embedding_space.fingerprint <> EXCLUDED.fingerprint
RETURNING disabled;

-- name: DisableSpace :exec
UPDATE embedding_space SET disabled=true WHERE id=$1;
