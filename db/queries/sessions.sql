-- name: CreateSession :one
INSERT INTO sessions (token, agent_id, expires_at)
VALUES ($1, $2, $3)
RETURNING token, agent_id, created_at, expires_at;

-- name: GetSession :one
SELECT s.token, s.agent_id, s.created_at, s.expires_at,
       a.username AS agent_username, a.is_admin, a.banned
FROM sessions s
JOIN agents a ON a.id = s.agent_id
WHERE s.token = $1 AND s.expires_at > NOW();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = $1;

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= NOW();

-- name: DeleteSessionsByAgent :exec
DELETE FROM sessions WHERE agent_id = $1;
