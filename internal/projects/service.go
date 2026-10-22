// Package projects manages projects and their membership.
package projects

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

const (
	maxNameLength = 100
	maxSlugLength = 50
)

var (
	validSlug   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	nonSlugRuns = regexp.MustCompile(`[^a-z0-9]+`)
)

type Project struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	Slug      string     `json:"slug"`
	Role      authz.Role `json:"role"`
	CreatedAt time.Time  `json:"created_at"`
}

var errSlugTaken = validate.Errors{"slug": "Another project already uses this slug."}

type Service struct {
	pool    *pgxpool.Pool
	queries *store.Queries
	authz   *authz.Authorizer
}

func NewService(pool *pgxpool.Pool, authorizer *authz.Authorizer) *Service {
	return &Service{pool: pool, queries: store.New(pool), authz: authorizer}
}

// CreateInput creates a project; Slug is derived from Name when empty
type CreateInput struct {
	Name string
	Slug string
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, in CreateInput) (Project, error) {
	name, slug, err := normalizeCreateInput(in)
	if err != nil {
		return Project{}, err
	}

	var created store.Project
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		created, err = q.CreateProject(ctx, store.CreateProjectParams{Name: name, Slug: slug, CreatedBy: &userID})
		if err != nil {
			return err
		}
		_, err = q.AddProjectMember(ctx, store.AddProjectMemberParams{
			ProjectID: created.ID, UserID: userID, Role: string(authz.RoleOwner),
		})
		return err
	})
	if database.IsUniqueViolation(err) {
		return Project{}, errSlugTaken
	}
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return toProject(created, authz.RoleOwner), nil
}

func (s *Service) Get(ctx context.Context, userID, projectID uuid.UUID) (Project, error) {
	role, err := s.authz.Require(ctx, userID, projectID, authz.ViewProject)
	if err != nil {
		return Project{}, err
	}
	p, err := s.queries.GetProject(ctx, projectID)
	if err != nil {
		return Project{}, fmt.Errorf("get project: %w", err)
	}
	return toProject(p, role), nil
}

type UpdateInput struct {
	Name *string
	Slug *string
}

func (s *Service) Update(ctx context.Context, userID, projectID uuid.UUID, in UpdateInput) (Project, error) {
	role, err := s.authz.Require(ctx, userID, projectID, authz.UpdateProject)
	if err != nil {
		return Project{}, err
	}
	params := store.UpdateProjectParams{ID: projectID}
	errs := validate.Errors{}
	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		validateName(errs, name)
		params.Name = &name
	}
	if in.Slug != nil {
		slug := strings.TrimSpace(*in.Slug)
		validateSlug(errs, slug)
		params.Slug = &slug
	}
	if err := errs.Err(); err != nil {
		return Project{}, err
	}

	p, err := s.queries.UpdateProject(ctx, params)
	if database.IsUniqueViolation(err) {
		return Project{}, errSlugTaken
	}
	if err != nil {
		return Project{}, fmt.Errorf("update project: %w", err)
	}
	return toProject(p, role), nil
}

// Delete removes a project; confirmSlug must match its slug
func (s *Service) Delete(ctx context.Context, userID, projectID uuid.UUID, confirmSlug string) error {
	if _, err := s.authz.Require(ctx, userID, projectID, authz.DeleteProject); err != nil {
		return err
	}
	p, err := s.queries.GetProject(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}
	if confirmSlug != p.Slug {
		return validate.Errors{"confirm": "Type the project slug to confirm deletion."}
	}
	err = s.queries.DeleteProject(ctx, projectID)
	if database.IsForeignKeyViolation(err) {
		return apperr.Conflict("project_has_servers", "Remove this project's servers before deleting it.")
	}
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	return nil
}

func (s *Service) List(ctx context.Context, userID uuid.UUID, search *string, limit, offset int32) ([]Project, error) {
	rows, err := s.queries.ListProjectsForUser(ctx, store.ListProjectsForUserParams{
		UserID: userID, Search: search, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	projects := make([]Project, len(rows))
	for i, row := range rows {
		projects[i] = toProject(row.Project, authz.Role(row.Role))
	}
	return projects, nil
}

func toProject(p store.Project, role authz.Role) Project {
	return Project{ID: p.ID, Name: p.Name, Slug: p.Slug, Role: role, CreatedAt: p.CreatedAt}
}

func normalizeCreateInput(in CreateInput) (name, slug string, err error) {
	errs := validate.Errors{}
	name = strings.TrimSpace(in.Name)
	validateName(errs, name)

	slug = strings.TrimSpace(in.Slug)
	if slug == "" {
		slug = slugify(name)
	} else {
		validateSlug(errs, slug)
	}
	return name, slug, errs.Err()
}

func validateName(errs validate.Errors, name string) {
	if name == "" || utf8.RuneCountInString(name) > maxNameLength {
		errs.Add("name", fmt.Sprintf("Name must be 1–%d characters.", maxNameLength))
	}
}

func validateSlug(errs validate.Errors, slug string) {
	if len(slug) > maxSlugLength || !validSlug.MatchString(slug) {
		errs.Add("slug", "Use lowercase letters, numbers and single hyphens.")
	}
}

// slugify derives a URL-safe slug; names without Latin letters or digits get a random one
func slugify(name string) string {
	slug := strings.Trim(nonSlugRuns.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(slug) > maxSlugLength {
		slug = strings.TrimRight(slug[:maxSlugLength], "-")
	}
	if slug == "" {
		b := make([]byte, 3)
		_, _ = rand.Read(b)
		slug = "project-" + hex.EncodeToString(b)
	}
	return slug
}
