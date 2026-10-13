-- +goose Up
-- A named target of an application: one server and one service configuration
-- (docs/15-deployments.md). config is validated JSON (internal/servicecfg);
-- there is no user-supplied Compose file.
CREATE TABLE environments (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id uuid NOT NULL REFERENCES applications (id) ON DELETE CASCADE,
    name           text NOT NULL CHECK (name ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    -- RESTRICT: a server cannot be removed while environments deploy to it.
    server_id      uuid NOT NULL REFERENCES servers (id) ON DELETE RESTRICT,
    auto_deploy    boolean NOT NULL DEFAULT false,
    config         jsonb NOT NULL,
    -- Copy of config.published_port so the database can refuse two
    -- environments claiming the same host port on one server.
    published_port integer CHECK (published_port BETWEEN 1024 AND 65535),
    created_by     uuid REFERENCES users (id) ON DELETE SET NULL,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT environments_application_name_key UNIQUE (application_id, name)
);

CREATE UNIQUE INDEX environments_server_port_key ON environments (server_id, published_port)
    WHERE published_port IS NOT NULL;
CREATE INDEX environments_server_idx ON environments (server_id);

-- +goose Down
DROP TABLE environments;
