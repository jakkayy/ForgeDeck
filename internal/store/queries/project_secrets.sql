-- name: CreateProjectSecret :one
INSERT INTO project_secrets (id, project_id, name, ciphertext, key_id, created_by)
VALUES (@id, @project_id, @name, @ciphertext, @key_id, @created_by)
RETURNING *;

-- name: ReplaceProjectSecret :one
UPDATE project_secrets
SET ciphertext = @ciphertext, key_id = @key_id, version = version + 1, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: GetProjectSecretByName :one
SELECT * FROM project_secrets WHERE project_id = $1 AND name = $2;

-- name: ListProjectSecrets :many
SELECT * FROM project_secrets WHERE project_id = $1 ORDER BY name;

-- name: ListProjectSecretsByNames :many
SELECT * FROM project_secrets WHERE project_id = @project_id AND name = ANY(@names::text[]) ORDER BY name;

-- name: DeleteProjectSecret :exec
DELETE FROM project_secrets WHERE id = $1;

-- name: EnvironmentsUsingSecret :many
-- Environments of the project whose configuration lists the secret.
SELECT environments.name FROM environments
JOIN applications ON applications.id = environments.application_id
WHERE applications.project_id = @project_id
  AND environments.config->'secrets' ? @secret_name::text
ORDER BY environments.name;
