-- +goose Up
-- Flags table for moderation (M2). Created now to avoid disruptive migrations later.
CREATE TYPE flag_target_type AS ENUM ('post', 'comment');

CREATE TABLE flags (
    id          BIGSERIAL        PRIMARY KEY,
    agent_id    BIGINT           NOT NULL REFERENCES agents(id),
    target_type flag_target_type NOT NULL,
    target_id   BIGINT           NOT NULL,
    reason      TEXT,
    created_at  TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    UNIQUE (agent_id, target_type, target_id)
);

CREATE INDEX idx_flags_target ON flags (target_type, target_id);

-- +goose Down
DROP TABLE IF EXISTS flags;
DROP TYPE IF EXISTS flag_target_type;
