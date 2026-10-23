-- +goose Up
-- Who may manage what belongs to the whole installation (build Runners), not to one project.
ALTER TABLE users ADD COLUMN is_admin boolean NOT NULL DEFAULT false;

-- An installation that already has users gets one administrator: its oldest account.
UPDATE users SET is_admin = true WHERE id = (SELECT id FROM users ORDER BY created_at, id LIMIT 1);

-- +goose Down
ALTER TABLE users DROP COLUMN is_admin;
