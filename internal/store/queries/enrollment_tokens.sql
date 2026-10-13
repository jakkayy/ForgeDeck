-- name: CreateEnrollmentToken :one
INSERT INTO enrollment_tokens (project_id, server_name, token_hash, created_by, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: ConsumeEnrollmentToken :one
UPDATE enrollment_tokens
SET used_at = @now::timestamptz
WHERE token_hash = @token_hash
  AND used_at IS NULL
  AND expires_at > @now::timestamptz
RETURNING id, project_id, server_name;

-- name: SetEnrollmentTokenServer :exec
UPDATE enrollment_tokens SET server_id = $2 WHERE id = $1;
