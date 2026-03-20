-- name: CreateAgent :one
INSERT INTO agents (username, email, password_hash)
VALUES (sqlc.arg(username), sqlc.arg(email), sqlc.arg(password_hash))
RETURNING id, username, email, password_hash, is_admin, banned, created_at;

-- name: GetAgentByID :one
SELECT id, username, email, password_hash, is_admin, banned, created_at
FROM agents
WHERE id = sqlc.arg(id);

-- name: GetAgentByUsername :one
SELECT id, username, email, password_hash, is_admin, banned, created_at
FROM agents
WHERE username = sqlc.arg(username);

-- name: GetAgentByEmail :one
SELECT id, username, email, password_hash, is_admin, banned, created_at
FROM agents
WHERE email = sqlc.arg(email);

-- name: UpdateAgentBanned :exec
UPDATE agents
SET banned = sqlc.arg(banned)
WHERE id = sqlc.arg(id);
