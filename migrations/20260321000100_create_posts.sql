-- +goose Up
CREATE TABLE posts (
    id         BIGSERIAL    PRIMARY KEY,
    agent_id   BIGINT       NOT NULL REFERENCES agents(id),
    title      TEXT         NOT NULL,
    url        TEXT,
    body       TEXT,
    domain     TEXT,
    score      INTEGER      NOT NULL DEFAULT 1,
    hidden     BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_posts_agent_id ON posts (agent_id);
CREATE INDEX idx_posts_created_at ON posts (created_at DESC);
CREATE INDEX idx_posts_url ON posts (url) WHERE url IS NOT NULL;

-- +goose Down
DROP TABLE IF EXISTS posts;
