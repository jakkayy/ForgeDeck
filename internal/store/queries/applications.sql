-- name: CreateApplication :one
INSERT INTO applications (
    project_id, name, git_connection_id, repository_id, repository_full_name,
    branch, dockerfile_path, build_context, test_command, provider_webhook_id, created_by
) VALUES (
    @project_id, @name, @git_connection_id, @repository_id, @repository_full_name,
    @branch, @dockerfile_path, @build_context, @test_command, @provider_webhook_id, @created_by
)
RETURNING *;

-- name: GetApplication :one
SELECT * FROM applications WHERE id = $1;

-- name: ListApplications :many
SELECT * FROM applications
WHERE project_id = @project_id
ORDER BY name, id
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateApplication :one
UPDATE applications
SET name            = COALESCE(sqlc.narg(name), name),
    branch          = COALESCE(sqlc.narg(branch), branch),
    dockerfile_path = COALESCE(sqlc.narg(dockerfile_path), dockerfile_path),
    build_context   = COALESCE(sqlc.narg(build_context), build_context),
    test_command    = CASE WHEN @set_test_command::boolean THEN sqlc.narg(test_command) ELSE test_command END,
    updated_at      = now()
WHERE id = @id
RETURNING *;

-- name: DeleteApplication :exec
DELETE FROM applications WHERE id = $1;

-- name: ApplicationsForGitHubPush :many
SELECT applications.* FROM applications
JOIN git_connections ON git_connections.id = applications.git_connection_id
WHERE git_connections.installation_id = @installation_id
  AND applications.repository_id = @repository_id
  AND applications.branch = @branch;

-- name: ApplicationsForGitLabPush :many
SELECT * FROM applications
WHERE git_connection_id = @git_connection_id
  AND repository_id = @repository_id
  AND branch = @branch;
