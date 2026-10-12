-- +goose Up
-- citext gives case-insensitive uniqueness for emails and slugs.
CREATE EXTENSION IF NOT EXISTS citext;

-- +goose Down
DROP EXTENSION IF EXISTS citext;
