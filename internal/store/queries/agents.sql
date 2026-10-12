-- name: CreateAgent :one
INSERT INTO agents (server_id, credential_hash)
VALUES ($1, $2)
RETURNING *;

-- name: GetAgentByCredentialHash :one
SELECT agents.id, agents.server_id, servers.project_id
FROM agents
JOIN servers ON servers.id = agents.server_id
WHERE agents.credential_hash = $1;
