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

// Project is a project as seen by one member, including their role.
type Project struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	Slug      string     `json:"slug"`
	Role      authz.Role `json:"role"`
	CreatedAt time.Time  `json:"created_at"`
}

// Service implements project operations.
type Service struct {
	pool    *pgxpool.Pool
	queries *store.Queries
}

// NewService returns a project Service.
func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, queries: store.New(pool)}
}

// CreateInput is the data needed to create a project. Slug is optional and
// derived from Name when empty.
type CreateInput struct {
	Name string
	Slug string
}

// Create stores a new project with userID as its owner.
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
		return q.AddProjectMember(ctx, store.AddProjectMemberParams{
			ProjectID: created.ID, UserID: userID, Role: string(authz.RoleOwner),
		})
	})
	if database.IsUniqueViolation(err) {
		return Project{}, validate.Errors{"slug": "Another project already uses this slug."}
	}
	if err != nil {
		return Project{}, fmt.Errorf("create project: %w", err)
	}
	return toProject(created, authz.RoleOwner), nil
}

// List returns the projects userID belongs to, ordered by name.
func (s *Service) List(ctx context.Context, userID uuid.UUID, limit, offset int32) ([]Project, error) {
	rows, err := s.queries.ListProjectsForUser(ctx, store.ListProjectsForUserParams{
		UserID: userID, PageLimit: limit, PageOffset: offset,
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
	if name == "" || utf8.RuneCountInString(name) > maxNameLength {
		errs.Add("name", fmt.Sprintf("Name must be 1–%d characters.", maxNameLength))
	}

	slug = strings.TrimSpace(in.Slug)
	switch {
	case slug == "":
		slug = slugify(name)
	case len(slug) > maxSlugLength || !validSlug.MatchString(slug):
		errs.Add("slug", "Use lowercase letters, numbers and single hyphens.")
	}
	return name, slug, errs.Err()
}

// slugify derives a URL-safe slug. Names without Latin letters or digits
// (for example Thai names) get a random slug instead.
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
