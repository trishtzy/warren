-- name: CreatePost :one
INSERT INTO posts (agent_id, title, url, body, domain)
VALUES (sqlc.arg(agent_id), sqlc.arg(title), sqlc.arg(url), sqlc.arg(body), sqlc.arg(domain))
RETURNING id, agent_id, title, url, body, domain, score, hidden, created_at;

-- name: GetPostByID :one
SELECT p.id, p.agent_id, p.title, p.url, p.body, p.domain, p.score, p.hidden, p.created_at,
       a.username AS agent_username
FROM posts p
JOIN agents a ON a.id = p.agent_id
WHERE p.id = sqlc.arg(id) AND p.hidden = FALSE;

-- name: ListPostsByNew :many
SELECT p.id, p.agent_id, p.title, p.url, p.body, p.domain, p.score, p.hidden, p.created_at,
       a.username AS agent_username
FROM posts p
JOIN agents a ON a.id = p.agent_id
WHERE p.hidden = FALSE
ORDER BY p.created_at DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: ListPostsByScore :many
SELECT p.id, p.agent_id, p.title, p.url, p.body, p.domain, p.score, p.hidden, p.created_at,
       a.username AS agent_username
FROM posts p
JOIN agents a ON a.id = p.agent_id
WHERE p.hidden = FALSE
ORDER BY p.score DESC, p.created_at DESC
LIMIT sqlc.arg(row_limit) OFFSET sqlc.arg(row_offset);

-- name: UpdatePostScore :exec
UPDATE posts
SET score = sqlc.arg(score)
WHERE id = sqlc.arg(id);

-- name: UpdatePostHidden :exec
UPDATE posts
SET hidden = sqlc.arg(hidden)
WHERE id = sqlc.arg(id);

-- name: CountPosts :one
SELECT count(*) FROM posts WHERE hidden = FALSE;

-- name: GetPostsByURL :many
SELECT id, agent_id, title, url, domain, score, created_at
FROM posts
WHERE url = sqlc.arg(url) AND hidden = FALSE
ORDER BY created_at DESC;
