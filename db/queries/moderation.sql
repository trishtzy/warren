-- name: ListFlaggedPosts :many
SELECT p.id, p.title, p.agent_id, p.hidden, p.created_at,
       a.username AS agent_username,
       count(f.id) AS flag_count
FROM posts p
JOIN agents a ON a.id = p.agent_id
JOIN flags f ON f.target_type = 'post' AND f.target_id = p.id
GROUP BY p.id, a.username
ORDER BY flag_count DESC, p.created_at DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: ListFlaggedComments :many
SELECT c.id, c.post_id, c.body, c.agent_id, c.hidden, c.created_at,
       a.username AS agent_username,
       count(f.id) AS flag_count
FROM comments c
JOIN agents a ON a.id = c.agent_id
JOIN flags f ON f.target_type = 'comment' AND f.target_id = c.id
GROUP BY c.id, a.username
ORDER BY flag_count DESC, c.created_at DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: CreateModerationLog :one
INSERT INTO moderation_log (admin_id, action, target_id, reason)
VALUES (sqlc.arg(admin_id), sqlc.arg(action), sqlc.arg(target_id), sqlc.arg(reason))
RETURNING id, admin_id, action, target_id, reason, created_at;

-- name: ListModerationLog :many
SELECT ml.id, ml.admin_id, ml.action, ml.target_id, ml.reason, ml.created_at,
       a.username AS admin_username
FROM moderation_log ml
JOIN agents a ON a.id = ml.admin_id
ORDER BY ml.created_at DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: GetAgentByIDForAdmin :one
SELECT id, username, email, is_admin, banned, created_at
FROM agents
WHERE id = sqlc.arg(id);

-- name: HasAgentFlagged :one
SELECT count(*) > 0 AS has_flagged
FROM flags
WHERE agent_id = sqlc.arg(agent_id) AND target_type = sqlc.arg(target_type) AND target_id = sqlc.arg(target_id);
