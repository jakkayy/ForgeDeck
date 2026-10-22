// Package authz decides what a project member may do, based on their role
package authz

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// Role is a project member's role
type Role string

const (
	RoleViewer    Role = "viewer"
	RoleDeveloper Role = "developer"
	RoleAdmin     Role = "admin"
	RoleOwner     Role = "owner"
)

var roleRank = map[Role]int{RoleViewer: 1, RoleDeveloper: 2, RoleAdmin: 3, RoleOwner: 4}

// ParseRole validates a role name
func ParseRole(s string) (Role, bool) {
	role := Role(s)
	_, ok := roleRank[role]
	return role, ok
}

// Permission is an action on a project or its resources
type Permission string

const (
	ViewProject      Permission = "project.view"
	OperateContainer Permission = "container.operate"
	ViewLogs         Permission = "container.logs"
	RunBuilds        Permission = "builds.run"
	Deploy           Permission = "deployments.run"
	UpdateProject    Permission = "project.update"
	ManageMembers    Permission = "members.manage"
	ManageServers    Permission = "servers.manage"
	ManageGit        Permission = "git.manage"
	ManageApps       Permission = "applications.manage"
	ViewAudit        Permission = "audit.view"
	DeleteProject    Permission = "project.delete"
)

// minimumRole lists the lowest role granted each permission
var minimumRole = map[Permission]Role{
	ViewProject:      RoleViewer,
	OperateContainer: RoleDeveloper,
	ViewLogs:         RoleDeveloper,
	RunBuilds:        RoleDeveloper,
	Deploy:           RoleDeveloper,
	UpdateProject:    RoleAdmin,
	ManageMembers:    RoleAdmin,
	ManageServers:    RoleAdmin,
	ManageGit:        RoleAdmin,
	ManageApps:       RoleAdmin,
	ViewAudit:        RoleAdmin,
	DeleteProject:    RoleOwner,
}

// Can reports whether role grants permission
func (r Role) Can(p Permission) bool {
	minimum, ok := minimumRole[p]
	return ok && roleRank[r] >= roleRank[minimum]
}

// Outranks reports whether r is strictly higher than other
func (r Role) Outranks(other Role) bool { return roleRank[r] > roleRank[other] }

// ErrProjectNotFound covers unknown and non-member projects so membership cannot be probed
var ErrProjectNotFound = apperr.NotFound("Project not found.")

// Authorizer checks a user's permission in a project
type Authorizer struct {
	queries *store.Queries
}

// New returns an Authorizer
func New(queries *store.Queries) *Authorizer {
	return &Authorizer{queries: queries}
}

// Require returns the user's role when it grants permission in projectID
func (a *Authorizer) Require(ctx context.Context, userID, projectID uuid.UUID, p Permission) (Role, error) {
	value, err := a.queries.GetMemberRole(ctx, store.GetMemberRoleParams{ProjectID: projectID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrProjectNotFound
	}
	if err != nil {
		return "", fmt.Errorf("load member role: %w", err)
	}
	role := Role(value)
	if !role.Can(p) {
		return role, apperr.Forbidden("Your role in this project does not allow this action.")
	}
	return role, nil
}
