-- +goose Up
CREATE TYPE moderation_action AS ENUM ('hide_post', 'unhide_post', 'hide_comment', 'unhide_comment', 'ban_agent', 'unban_agent');

CREATE TABLE moderation_log (
    id          BIGSERIAL          PRIMARY KEY,
    admin_id    BIGINT             NOT NULL REFERENCES agents(id),
    action      moderation_action  NOT NULL,
    target_id   BIGINT             NOT NULL,
    reason      TEXT,
    created_at  TIMESTAMPTZ        NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_moderation_log_admin_id ON moderation_log (admin_id);
CREATE INDEX idx_moderation_log_created_at ON moderation_log (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS moderation_log;
DROP TYPE IF EXISTS moderation_action;
