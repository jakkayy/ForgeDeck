-- +goose Up
-- Set when retention removed a finished job's log lines.
ALTER TABLE jobs ADD COLUMN logs_expired_at timestamptz;

CREATE INDEX jobs_finished_idx ON jobs (finished_at) WHERE finished_at IS NOT NULL;

-- +goose Down
DROP INDEX jobs_finished_idx;
ALTER TABLE jobs DROP COLUMN logs_expired_at;
