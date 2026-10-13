-- +goose Up
ALTER TABLE jobs ADD COLUMN application_id uuid REFERENCES applications (id) ON DELETE CASCADE;
CREATE INDEX jobs_application_history_idx ON jobs (application_id, created_at DESC)
    WHERE application_id IS NOT NULL;

-- +goose Down
ALTER TABLE jobs DROP COLUMN application_id;
