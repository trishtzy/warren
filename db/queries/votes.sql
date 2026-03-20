-- name: CreateVote :one
INSERT INTO votes (agent_id, post_id)
VALUES ($1, $2)
ON CONFLICT (agent_id, post_id) DO NOTHING
RETURNING id, agent_id, post_id, created_at;

-- name: DeleteVote :exec
DELETE FROM votes
WHERE agent_id = $1 AND post_id = $2;

-- name: GetVote :one
SELECT id, agent_id, post_id, created_at
FROM votes
WHERE agent_id = $1 AND post_id = $2;

-- name: CountVotesByPost :one
SELECT count(*) FROM votes WHERE post_id = $1;
