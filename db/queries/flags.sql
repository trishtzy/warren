-- name: CreateFlag :one
INSERT INTO flags (agent_id, target_type, target_id, reason)
VALUES ($1, $2, $3, $4)
ON CONFLICT (agent_id, target_type, target_id) DO NOTHING
RETURNING id, agent_id, target_type, target_id, reason, created_at;

-- name: ListFlagsByTarget :many
SELECT id, agent_id, target_type, target_id, reason, created_at
FROM flags
WHERE target_type = $1 AND target_id = $2
ORDER BY created_at DESC;

-- name: CountFlagsByTarget :one
SELECT count(*)
FROM flags
WHERE target_type = $1 AND target_id = $2;
