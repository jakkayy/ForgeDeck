-- +goose Up
-- An append-only record of who did what. Rows are never updated; they are
-- removed only by the retention job. project_id has no foreign key on
-- purpose: deleting a project must not erase the record of who deleted it.
CREATE TABLE audit_logs (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id    uuid,
    actor_user_id uuid REFERENCES users (id) ON DELETE SET NULL,
    -- Who, as text, so the record survives the user: an email address, or
    -- "agent"/"runner"/"system" for non-human actors.
    actor         text NOT NULL,
    action        text NOT NULL CHECK (action ~ '^[a-z_]+(\.[a-z_]+)+$'),
    target_type   text NOT NULL,
    target_id     text,
    target_name   text,
    -- Non-sensitive details only: never a secret, token, or password.
    metadata      jsonb NOT NULL DEFAULT '{}',
    request_id    text,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_project_idx ON audit_logs (project_id, created_at DESC, id DESC);
CREATE INDEX audit_logs_created_idx ON audit_logs (created_at);

-- +goose Down
DROP TABLE audit_logs;
