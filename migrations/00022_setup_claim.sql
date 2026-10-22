-- +goose Up
-- One row at most, inserted by whoever registers the first account from the
-- web. A second registration finds the row taken (or a user present) and is
-- refused, even if two arrive at the same instant.
CREATE TABLE setup_claim (
    singleton  boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    claimed_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE setup_claim;
