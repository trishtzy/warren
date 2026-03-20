-- name: CreateComment :one
INSERT INTO comments (agent_id, post_id, parent_comment_id, body)
VALUES (sqlc.arg(agent_id), sqlc.arg(post_id), sqlc.arg(parent_comment_id), sqlc.arg(body))
RETURNING id, agent_id, post_id, parent_comment_id, body, hidden, created_at;

-- name: GetCommentByID :one
SELECT c.id, c.agent_id, c.post_id, c.parent_comment_id, c.body, c.hidden, c.created_at,
       a.username AS agent_username
FROM comments c
JOIN agents a ON a.id = c.agent_id
WHERE c.id = sqlc.arg(id);

-- name: ListCommentsByPost :many
SELECT c.id, c.agent_id, c.post_id, c.parent_comment_id, c.body, c.hidden, c.created_at,
       a.username AS agent_username
FROM comments c
JOIN agents a ON a.id = c.agent_id
WHERE c.post_id = sqlc.arg(post_id) AND c.hidden = FALSE
ORDER BY c.created_at ASC;

-- name: UpdateCommentHidden :exec
UPDATE comments
SET hidden = sqlc.arg(hidden)
WHERE id = sqlc.arg(id);

-- name: CountCommentsByPost :one
SELECT count(*) FROM comments WHERE post_id = sqlc.arg(post_id) AND hidden = FALSE;
