-- +goose Up
-- Rotating an Agent's credential: the new one takes effect at once and the
-- one it replaces keeps working for a short grace period, so an Agent that
-- loses the rotation response (or crashes before saving it) is not locked out.
ALTER TABLE agents
    ADD COLUMN previous_credential_hash bytea UNIQUE,
    ADD COLUMN previous_expires_at      timestamptz,
    ADD COLUMN credential_rotated_at    timestamptz NOT NULL DEFAULT now(),
    -- Set when an admin asks for a rotation; the next heartbeat tells the Agent.
    ADD COLUMN rotation_requested_at    timestamptz,
    ADD CONSTRAINT agents_previous_credential_check
        CHECK ((previous_credential_hash IS NULL) = (previous_expires_at IS NULL));

-- +goose Down
ALTER TABLE agents
    DROP CONSTRAINT agents_previous_credential_check,
    DROP COLUMN previous_credential_hash,
    DROP COLUMN previous_expires_at,
    DROP COLUMN credential_rotated_at,
    DROP COLUMN rotation_requested_at;
