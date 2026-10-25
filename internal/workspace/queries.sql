-- name: CreateUser :one
INSERT INTO app_user (auth_sub, email, name) VALUES ($1, $2, $3)
ON CONFLICT (auth_sub) DO NOTHING
RETURNING id;

-- name: UserBySubject :one
SELECT id FROM app_user WHERE auth_sub = $1;

-- name: CreateWorkspace :one
INSERT INTO workspace (name) VALUES ($1) RETURNING id;

-- name: AddOwner :exec
INSERT INTO membership (workspace_id, user_id, role) VALUES ($1, $2, 'owner');

-- name: MembershipsForUser :many
SELECT workspace_id, role FROM membership WHERE user_id = $1 ORDER BY workspace_id LIMIT 2;

-- name: MembershipForWorkspace :one
SELECT role FROM membership WHERE user_id = $1 AND workspace_id = $2;
