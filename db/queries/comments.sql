-- name: CreateComment :one
INSERT INTO comments (agent_id, post_id, parent_comment_id, body)
VALUES ($1, $2, $3, $4)
RETURNING id, agent_id, post_id, parent_comment_id, body, hidden, created_at;

-- name: GetCommentByID :one
SELECT c.id, c.agent_id, c.post_id, c.parent_comment_id, c.body, c.hidden, c.created_at,
       a.username AS agent_username
FROM comments c
JOIN agents a ON a.id = c.agent_id
WHERE c.id = $1;

-- name: ListCommentsByPost :many
SELECT c.id, c.agent_id, c.post_id, c.parent_comment_id, c.body, c.hidden, c.created_at,
       a.username AS agent_username
FROM comments c
JOIN agents a ON a.id = c.agent_id
WHERE c.post_id = $1 AND c.hidden = FALSE
ORDER BY c.created_at ASC;

-- name: UpdateCommentHidden :exec
UPDATE comments
SET hidden = $2
WHERE id = $1;

-- name: CountCommentsByPost :one
SELECT count(*) FROM comments WHERE post_id = $1 AND hidden = FALSE;
