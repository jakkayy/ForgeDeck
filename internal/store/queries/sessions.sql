-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, created_at, last_seen_at, expires_at)
VALUES (@user_id, @token_hash, @now, @now, @expires_at)
RETURNING *;

-- name: GetActiveSession :one
SELECT
    sessions.id,
    sessions.user_id,
    sessions.created_at,
    sessions.last_seen_at,
    sessions.expires_at,
    users.email
FROM sessions
JOIN users ON users.id = sessions.user_id
WHERE sessions.token_hash = @token_hash
  AND sessions.expires_at > @now;

-- name: TouchSession :exec
UPDATE sessions
SET last_seen_at = @now, expires_at = @expires_at
WHERE id = @id;

-- name: DeleteSessionByTokenHash :exec
DELETE FROM sessions WHERE token_hash = $1;
