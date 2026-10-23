-- name: CreateRunner :one
INSERT INTO runners (name, credential_hash) VALUES ($1, $2) RETURNING *;

-- name: DeleteRunnerByName :execrows
DELETE FROM runners WHERE name = $1;

-- name: GetRunnerByCredentialHash :one
SELECT * FROM runners WHERE credential_hash = $1;

-- name: RecordRunnerHeartbeat :exec
UPDATE runners SET last_seen_at = @now::timestamptz, version = @version WHERE id = @id;

-- name: ListRunners :many
SELECT * FROM runners ORDER BY name;

-- name: DeleteRunnerByID :execrows
DELETE FROM runners WHERE id = $1;

-- name: CountRunnersByLiveness :one
-- Online means a heartbeat since the cutoff the caller passes.
SELECT count(*) AS total,
       count(*) FILTER (WHERE last_seen_at >= @online_after::timestamptz) AS online
FROM runners;

-- name: GetRunner :one
SELECT * FROM runners WHERE id = $1;
