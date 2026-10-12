-- name: GetMemberRole :one
SELECT role FROM project_members
WHERE project_id = $1 AND user_id = $2;
