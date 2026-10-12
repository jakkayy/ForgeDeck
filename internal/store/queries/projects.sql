-- name: CreateProject :one
INSERT INTO projects (name, slug, created_by)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListProjectsForUser :many
SELECT sqlc.embed(projects), project_members.role
FROM projects
JOIN project_members ON project_members.project_id = projects.id
WHERE project_members.user_id = @user_id
ORDER BY projects.name, projects.id
LIMIT @page_limit OFFSET @page_offset;
