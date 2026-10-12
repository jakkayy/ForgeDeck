-- name: GetMemberRole :one
SELECT role FROM project_members
WHERE project_id = $1 AND user_id = $2;

-- name: AddProjectMember :one
INSERT INTO project_members (project_id, user_id, role)
VALUES ($1, $2, $3)
RETURNING created_at;

-- name: ListProjectMembers :many
SELECT project_members.user_id, users.email, project_members.role, project_members.created_at
FROM project_members
JOIN users ON users.id = project_members.user_id
WHERE project_members.project_id = @project_id
ORDER BY users.email
LIMIT @page_limit OFFSET @page_offset;

-- name: CountProjectOwners :one
SELECT count(*) FROM project_members
WHERE project_id = $1 AND role = 'owner';

-- name: RemoveProjectMember :exec
DELETE FROM project_members
WHERE project_id = $1 AND user_id = $2;
