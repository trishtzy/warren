-- +goose Up

-- Covers the WHERE hidden = FALSE filter on posts and provides pre-sorted
-- score + created_at data. The ranking ORDER BY uses a computed expression
-- so Postgres cannot use this index for sorting directly, but it still
-- speeds up the filtered scan and benefits the /new chronological listing.
CREATE INDEX idx_posts_score_created ON posts (score, created_at DESC) WHERE hidden = FALSE;

-- Speeds up the correlated comment_count subquery in post listing queries.
-- The existing idx_comments_post_id covers all rows; this partial index
-- narrows to visible comments only.
CREATE INDEX idx_comments_post_id_visible ON comments (post_id) WHERE hidden = FALSE;

-- +goose Down
DROP INDEX IF EXISTS idx_comments_post_id_visible;
DROP INDEX IF EXISTS idx_posts_score_created;
