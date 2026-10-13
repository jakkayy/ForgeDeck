-- +goose Up
-- A deployable component built from one repository and branch.
CREATE TABLE applications (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id           uuid NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name                 text NOT NULL CHECK (char_length(name) BETWEEN 1 AND 100),
    -- RESTRICT: a connection cannot be removed while applications build from it.
    git_connection_id    uuid NOT NULL REFERENCES git_connections (id) ON DELETE RESTRICT,
    repository_id        text NOT NULL,
    repository_full_name text NOT NULL,
    branch               text NOT NULL,
    dockerfile_path      text NOT NULL DEFAULT 'Dockerfile',
    build_context        text NOT NULL DEFAULT '.',
    test_command         text,
    -- ID of the webhook ForgeDeck registered on GitLab; GitHub App webhooks are app-wide.
    provider_webhook_id  text,
    created_by           uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

-- Webhook routing: which applications build from a repository.
CREATE INDEX applications_repository_idx ON applications (git_connection_id, repository_id);

-- +goose Down
DROP TABLE applications;
