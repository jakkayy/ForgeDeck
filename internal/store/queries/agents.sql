-- name: CreateAgent :one
INSERT INTO agents (server_id, credential_hash)
VALUES ($1, $2)
RETURNING *;
