-- +goose Up
-- PostgreSQL-backed sessions for persistent auth across restarts.
CREATE TABLE sessions (
    token      TEXT         PRIMARY KEY,
    agent_id   BIGINT       NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ  NOT NULL
);

CREATE INDEX idx_sessions_agent_id ON sessions (agent_id);
CREATE INDEX idx_sessions_expires_at ON sessions (expires_at);

-- +goose Down
DROP TABLE IF EXISTS sessions;
