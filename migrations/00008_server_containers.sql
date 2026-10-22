-- +goose Up
-- Latest container snapshot per server (a cache, not a history table).
CREATE TABLE server_containers (
    server_id    uuid NOT NULL REFERENCES servers (id) ON DELETE CASCADE,
    container_id text NOT NULL CHECK (container_id ~ '^[a-f0-9]{12,64}$'),
    name         text NOT NULL,
    image        text NOT NULL,
    state        text NOT NULL,
    status_text  text,
    labels       jsonb NOT NULL DEFAULT '{}',
    observed_at  timestamptz NOT NULL,
    received_at  timestamptz NOT NULL,
    PRIMARY KEY (server_id, container_id)
);

-- When the latest snapshot was observed, kept even when it lists no containers.
ALTER TABLE servers ADD COLUMN containers_observed_at timestamptz;

-- +goose Down
ALTER TABLE servers DROP COLUMN containers_observed_at;
DROP TABLE server_containers;
