-- +goose Up
-- One row per provider delivery. The unique key drops redeliveries, which
-- GitHub and GitLab send on timeouts and manual retries.
CREATE TABLE webhook_events (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider          text NOT NULL CHECK (provider IN ('github', 'gitlab')),
    delivery_id       text NOT NULL,
    event_type        text NOT NULL,
    git_connection_id uuid REFERENCES git_connections (id) ON DELETE SET NULL,
    received_at       timestamptz NOT NULL DEFAULT now(),
    -- What ForgeDeck did with it, e.g. "queued 2 builds" or "ignored: branch not tracked".
    result            text,
    UNIQUE (provider, delivery_id)
);

CREATE INDEX webhook_events_received_at_idx ON webhook_events (received_at);

-- +goose Down
DROP TABLE webhook_events;
