-- name: CreateUser :one
INSERT INTO users (email, password_hash)
VALUES ($1, $2)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: HasUsers :one
SELECT EXISTS (SELECT 1 FROM users);

-- name: CreateFirstUser :one
-- Creates the user only if there is none and nobody else is registering at the
-- same moment. One statement: a concurrent caller blocks on the claim row until
-- this one commits, then finds it taken and inserts nothing (no rows returned).
WITH claim AS (
    INSERT INTO setup_claim DEFAULT VALUES ON CONFLICT DO NOTHING RETURNING singleton
)
INSERT INTO users (email, password_hash)
SELECT @email::citext, @password_hash::text
FROM claim
WHERE NOT EXISTS (SELECT 1 FROM users)
RETURNING *;
