-- name: CreateGitHubConnection :one
INSERT INTO git_connections (project_id, provider, display_name, base_url, installation_id, created_by)
VALUES (@project_id, 'github', @display_name, @base_url, @installation_id, @created_by)
ON CONFLICT (project_id, installation_id) DO UPDATE SET display_name = EXCLUDED.display_name
RETURNING *;

-- name: ListGitConnections :many
SELECT * FROM git_connections
WHERE project_id = $1
ORDER BY created_at;

-- name: GetGitConnection :one
SELECT * FROM git_connections WHERE id = $1;

-- name: DeleteGitConnection :exec
DELETE FROM git_connections WHERE id = $1;

-- name: CreateGitLabConnection :one
INSERT INTO git_connections (
    id, project_id, provider, display_name, base_url,
    token_ciphertext, token_key_id, webhook_ciphertext, webhook_key_id, created_by
) VALUES (
    @id, @project_id, 'gitlab', @display_name, @base_url,
    @token_ciphertext, @token_key_id, @webhook_ciphertext, @webhook_key_id, @created_by
)
RETURNING *;
