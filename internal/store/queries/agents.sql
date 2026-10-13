-- name: CreateAgent :one
INSERT INTO agents (server_id, credential_hash)
VALUES ($1, $2)
RETURNING *;

-- name: GetAgentByCredentialHash :one
-- The current credential, or the one it replaced while its grace period lasts.
SELECT agents.id, agents.server_id, servers.project_id,
       agents.credential_rotated_at, agents.rotation_requested_at,
       (agents.credential_hash <> @hash)::boolean AS is_previous
FROM agents
JOIN servers ON servers.id = agents.server_id
WHERE agents.credential_hash = @hash
   OR (agents.previous_credential_hash = @hash AND agents.previous_expires_at > @now::timestamptz);

-- name: RotateAgentCredential :execrows
-- Installs a new credential and keeps the one presented for a grace period.
UPDATE agents
SET previous_credential_hash = @presented_hash,
    previous_expires_at      = @grace_until::timestamptz,
    credential_hash          = @new_hash,
    credential_rotated_at    = @now::timestamptz,
    rotation_requested_at    = NULL
WHERE id = @id
  AND (credential_hash = @presented_hash
       OR (previous_credential_hash = @presented_hash AND previous_expires_at > @now::timestamptz));

-- name: RequestAgentRotation :execrows
UPDATE agents SET rotation_requested_at = COALESCE(rotation_requested_at, @now::timestamptz)
WHERE server_id = @server_id;
