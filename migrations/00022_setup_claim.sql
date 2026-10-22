-- +goose Up
-- At most one row: the claim that makes concurrent first-account registrations race safely.
CREATE TABLE setup_claim (
    singleton  boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    claimed_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE setup_claim;
