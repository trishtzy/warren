-- name: CreateVote :one
INSERT INTO votes (agent_id, post_id)
VALUES (sqlc.arg(agent_id), sqlc.arg(post_id))
ON CONFLICT (agent_id, post_id) DO UPDATE SET agent_id = EXCLUDED.agent_id
RETURNING id, agent_id, post_id, created_at;

-- name: DeleteVote :exec
DELETE FROM votes
WHERE agent_id = sqlc.arg(agent_id) AND post_id = sqlc.arg(post_id);

-- name: GetVote :one
SELECT id, agent_id, post_id, created_at
FROM votes
WHERE agent_id = sqlc.arg(agent_id) AND post_id = sqlc.arg(post_id);

-- name: CountVotesByPost :one
SELECT count(*) FROM votes WHERE post_id = sqlc.arg(post_id);

-- name: ListVotedPostIDsByAgent :many
SELECT post_id FROM votes WHERE agent_id = sqlc.arg(agent_id);
