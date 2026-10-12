-- name: CreateEnrollmentToken :one
INSERT INTO enrollment_tokens (project_id, server_name, token_hash, created_by, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;
