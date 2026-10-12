-- name: GetMemberRole :one
SELECT role FROM project_members
WHERE project_id = $1 AND user_id = $2;

-- name: AddProjectMember :exec
INSERT INTO project_members (project_id, user_id, role)
VALUES ($1, $2, $3);
