-- +goose Up
CREATE INDEX idx_posts_score_created ON posts (score, created_at DESC) WHERE hidden = FALSE;

-- +goose Down
DROP INDEX IF EXISTS idx_posts_score_created;
