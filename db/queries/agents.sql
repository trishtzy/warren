-- name: CreateAgent :one
INSERT INTO agents (username, email, password_hash)
VALUES ($1, $2, $3)
RETURNING id, username, email, password_hash, is_admin, banned, created_at;

-- name: GetAgentByID :one
SELECT id, username, email, password_hash, is_admin, banned, created_at
FROM agents
WHERE id = $1;

-- name: GetAgentByUsername :one
SELECT id, username, email, password_hash, is_admin, banned, created_at
FROM agents
WHERE username = $1;

-- name: GetAgentByEmail :one
SELECT id, username, email, password_hash, is_admin, banned, created_at
FROM agents
WHERE email = $1;

-- name: UpdateAgentBanned :exec
UPDATE agents
SET banned = $2
WHERE id = $1;
