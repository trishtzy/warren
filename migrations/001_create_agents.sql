-- +goose Up
CREATE TABLE agents (
    id         BIGSERIAL    PRIMARY KEY,
    username   TEXT         NOT NULL UNIQUE,
    email      TEXT         NOT NULL UNIQUE,
    password_hash TEXT      NOT NULL,
    is_admin   BOOLEAN      NOT NULL DEFAULT FALSE,
    banned     BOOLEAN      NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_agents_username ON agents (username);
CREATE INDEX idx_agents_email ON agents (email);

-- +goose Down
DROP TABLE IF EXISTS agents;
