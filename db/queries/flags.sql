-- name: CreateFlag :one
INSERT INTO flags (agent_id, target_type, target_id, reason)
VALUES (sqlc.arg(agent_id), sqlc.arg(target_type), sqlc.arg(target_id), sqlc.arg(reason))
ON CONFLICT (agent_id, target_type, target_id) DO NOTHING
RETURNING id, agent_id, target_type, target_id, reason, created_at;

-- name: ListFlagsByTarget :many
SELECT id, agent_id, target_type, target_id, reason, created_at
FROM flags
WHERE target_type = sqlc.arg(target_type) AND target_id = sqlc.arg(target_id)
ORDER BY created_at DESC;

-- name: CountFlagsByTarget :one
SELECT count(*)
FROM flags
WHERE target_type = sqlc.arg(target_type) AND target_id = sqlc.arg(target_id);
