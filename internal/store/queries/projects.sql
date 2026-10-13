-- name: CreateProject :one
INSERT INTO projects (name, slug, created_by)
VALUES ($1, $2, $3)
RETURNING *;

-- name: ListProjectsForUser :many
SELECT sqlc.embed(projects), project_members.role
FROM projects
JOIN project_members ON project_members.project_id = projects.id
WHERE project_members.user_id = @user_id
  AND (sqlc.narg(search)::text IS NULL
       OR projects.name ILIKE '%' || sqlc.narg(search)::text || '%'
       OR projects.slug ILIKE '%' || sqlc.narg(search)::text || '%')
ORDER BY projects.name, projects.id
LIMIT @page_limit OFFSET @page_offset;

-- name: GetProject :one
SELECT * FROM projects WHERE id = $1;

-- name: UpdateProject :one
UPDATE projects
SET name = COALESCE(sqlc.narg(name), name),
    slug = COALESCE(sqlc.narg(slug), slug),
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: DeleteProject :exec
DELETE FROM projects WHERE id = $1;

-- name: LockProject :exec
SELECT id FROM projects WHERE id = $1 FOR UPDATE;
