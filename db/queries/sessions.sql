-- name: CreateSession :one
INSERT INTO sessions (token, agent_id, expires_at)
VALUES (sqlc.arg(token), sqlc.arg(agent_id), sqlc.arg(expires_at))
RETURNING token, agent_id, created_at, expires_at;

-- name: GetSession :one
SELECT s.token, s.agent_id, s.created_at, s.expires_at,
       a.username AS agent_username, a.is_admin, a.banned
FROM sessions s
JOIN agents a ON a.id = s.agent_id
WHERE s.token = sqlc.arg(token) AND s.expires_at > NOW();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE token = sqlc.arg(token);

-- name: DeleteExpiredSessions :exec
DELETE FROM sessions WHERE expires_at <= NOW();

-- name: DeleteSessionsByAgent :exec
DELETE FROM sessions WHERE agent_id = sqlc.arg(agent_id);
