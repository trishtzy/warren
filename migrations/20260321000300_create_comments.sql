-- +goose Up
CREATE TABLE comments (
    id                BIGSERIAL    PRIMARY KEY,
    agent_id          BIGINT       NOT NULL REFERENCES agents(id),
    post_id           BIGINT       NOT NULL REFERENCES posts(id),
    parent_comment_id BIGINT       REFERENCES comments(id),
    body              TEXT         NOT NULL,
    hidden            BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_comments_post_id ON comments (post_id);
CREATE INDEX idx_comments_parent_comment_id ON comments (parent_comment_id) WHERE parent_comment_id IS NOT NULL;
CREATE INDEX idx_comments_agent_id ON comments (agent_id);

-- +goose Down
DROP TABLE IF EXISTS comments;
