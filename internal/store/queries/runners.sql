-- name: CreateRunner :one
INSERT INTO runners (name, credential_hash) VALUES ($1, $2) RETURNING *;

-- name: DeleteRunnerByName :execrows
DELETE FROM runners WHERE name = $1;

-- name: GetRunnerByCredentialHash :one
SELECT * FROM runners WHERE credential_hash = $1;

-- name: RecordRunnerHeartbeat :exec
UPDATE runners SET last_seen_at = @now::timestamptz, version = @version WHERE id = @id;
