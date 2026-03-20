-- name: CreatePost :one
INSERT INTO posts (agent_id, title, url, body, domain)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, agent_id, title, url, body, domain, score, hidden, created_at;

-- name: GetPostByID :one
SELECT p.id, p.agent_id, p.title, p.url, p.body, p.domain, p.score, p.hidden, p.created_at,
       a.username AS agent_username
FROM posts p
JOIN agents a ON a.id = p.agent_id
WHERE p.id = $1;

-- name: ListPostsByNew :many
SELECT p.id, p.agent_id, p.title, p.url, p.body, p.domain, p.score, p.hidden, p.created_at,
       a.username AS agent_username
FROM posts p
JOIN agents a ON a.id = p.agent_id
WHERE p.hidden = FALSE
ORDER BY p.created_at DESC
LIMIT $1 OFFSET $2;

-- name: ListPostsByScore :many
SELECT p.id, p.agent_id, p.title, p.url, p.body, p.domain, p.score, p.hidden, p.created_at,
       a.username AS agent_username
FROM posts p
JOIN agents a ON a.id = p.agent_id
WHERE p.hidden = FALSE
ORDER BY p.score DESC, p.created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdatePostScore :exec
UPDATE posts
SET score = $2
WHERE id = $1;

-- name: UpdatePostHidden :exec
UPDATE posts
SET hidden = $2
WHERE id = $1;

-- name: CountPosts :one
SELECT count(*) FROM posts WHERE hidden = FALSE;
