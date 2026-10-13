package projects

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

// Member is a user's membership in a project.
type Member struct {
	UserID  uuid.UUID  `json:"user_id"`
	Email   string     `json:"email"`
	Role    authz.Role `json:"role"`
	AddedAt time.Time  `json:"added_at"`
}

var (
	errMemberNotFound   = apperr.NotFound("Member not found.")
	errRoleAboveOwn     = apperr.Forbidden("You cannot grant or remove a role higher than your own.")
	errLastOwner        = apperr.Conflict("last_owner", "A project must keep at least one owner.")
	errAlreadyMember    = apperr.Conflict("already_member", "This user is already a member of the project.")
	errUnknownMemberKey = validate.Errors{"email": "No ForgeDeck user has this email. Create the account first."}
)

// ListMembers returns the members of a project the user belongs to.
func (s *Service) ListMembers(ctx context.Context, userID, projectID uuid.UUID, limit, offset int32) ([]Member, error) {
	if _, err := s.authz.Require(ctx, userID, projectID, authz.ViewProject); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListProjectMembers(ctx, store.ListProjectMembersParams{
		ProjectID: projectID, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	members := make([]Member, len(rows))
	for i, row := range rows {
		members[i] = Member{UserID: row.UserID, Email: row.Email, Role: authz.Role(row.Role), AddedAt: row.CreatedAt}
	}
	return members, nil
}

// AddMember gives an existing user a role in the project.
func (s *Service) AddMember(ctx context.Context, actorID, projectID uuid.UUID, email, roleName string) (Member, error) {
	actorRole, err := s.authz.Require(ctx, actorID, projectID, authz.ManageMembers)
	if err != nil {
		return Member{}, err
	}
	role, ok := authz.ParseRole(roleName)
	if !ok {
		return Member{}, validate.Errors{"role": "Choose owner, admin, developer or viewer."}
	}
	if role.Outranks(actorRole) {
		return Member{}, errRoleAboveOwn
	}

	user, err := s.queries.GetUserByEmail(ctx, strings.TrimSpace(email))
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, errUnknownMemberKey
	}
	if err != nil {
		return Member{}, fmt.Errorf("find user: %w", err)
	}

	addedAt, err := s.queries.AddProjectMember(ctx, store.AddProjectMemberParams{ProjectID: projectID, UserID: user.ID, Role: string(role)})
	if database.IsUniqueViolation(err) {
		return Member{}, errAlreadyMember
	}
	if err != nil {
		return Member{}, fmt.Errorf("add member: %w", err)
	}
	return Member{UserID: user.ID, Email: user.Email, Role: role, AddedAt: addedAt}, nil
}

// RemoveMember revokes a user's access to the project. Members may always
// remove themselves, except the last owner.
func (s *Service) RemoveMember(ctx context.Context, actorID, projectID, memberID uuid.UUID) error {
	var actorRole authz.Role
	var err error
	if actorID == memberID {
		actorRole, err = s.authz.Require(ctx, actorID, projectID, authz.ViewProject)
	} else {
		actorRole, err = s.authz.Require(ctx, actorID, projectID, authz.ManageMembers)
	}
	if err != nil {
		return err
	}

	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		// Serialize membership changes per project so two concurrent removals
		// cannot both pass the last-owner check.
		if err := q.LockProject(ctx, projectID); err != nil {
			return fmt.Errorf("lock project: %w", err)
		}
		value, err := q.GetMemberRole(ctx, store.GetMemberRoleParams{ProjectID: projectID, UserID: memberID})
		if errors.Is(err, pgx.ErrNoRows) {
			return errMemberNotFound
		}
		if err != nil {
			return fmt.Errorf("load member: %w", err)
		}
		memberRole := authz.Role(value)
		if memberRole.Outranks(actorRole) {
			return errRoleAboveOwn
		}
		if memberRole == authz.RoleOwner {
			owners, err := q.CountProjectOwners(ctx, projectID)
			if err != nil {
				return fmt.Errorf("count owners: %w", err)
			}
			if owners <= 1 {
				return errLastOwner
			}
		}
		if err := q.RemoveProjectMember(ctx, store.RemoveProjectMemberParams{ProjectID: projectID, UserID: memberID}); err != nil {
			return fmt.Errorf("remove member: %w", err)
		}
		return nil
	})
}
