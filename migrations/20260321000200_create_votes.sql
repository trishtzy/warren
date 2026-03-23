-- +goose Up
CREATE TABLE votes (
    id         BIGSERIAL    PRIMARY KEY,
    agent_id   BIGINT       NOT NULL REFERENCES agents(id),
    post_id    BIGINT       NOT NULL REFERENCES posts(id),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (agent_id, post_id)
);

CREATE INDEX idx_votes_post_id ON votes (post_id);

-- +goose Down
DROP TABLE IF EXISTS votes;
