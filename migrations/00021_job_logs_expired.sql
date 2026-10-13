-- +goose Up
-- Set when a finished job's log lines were removed by the retention job, so
-- the dashboard can say why a job that did run has no output.
ALTER TABLE jobs ADD COLUMN logs_expired_at timestamptz;

-- The retention job looks for finished jobs by age.
CREATE INDEX jobs_finished_idx ON jobs (finished_at) WHERE finished_at IS NOT NULL;

-- +goose Down
DROP INDEX jobs_finished_idx;
ALTER TABLE jobs DROP COLUMN logs_expired_at;
