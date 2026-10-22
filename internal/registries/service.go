// Package registries manages the container registries a project pushes images to.
package registries

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/keyring"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

const (
	maxNameLength     = 100
	maxUsernameLength = 255
	maxPasswordLength = 4096
)

var (
	// errRegistryNotFound also covers registries in projects the user cannot see
	errRegistryNotFound = apperr.NotFound("Registry not found.")
	errNameTaken        = validate.Errors{"name": "Another registry in this project already uses this name."}

	// validHost is a hostname with optional port, as Docker expects (no scheme, no path)
	validHost = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?(:[0-9]{1,5})?$`)
)

// Registry is the API view of a registry connection; the password never leaves the server
type Registry struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}

type Credentials struct {
	Host     string
	Username string
	Password string
}

type Service struct {
	queries *store.Queries
	authz   *authz.Authorizer
	keyring *keyring.Keyring
}

func NewService(queries *store.Queries, authorizer *authz.Authorizer, ring *keyring.Keyring) *Service {
	return &Service{queries: queries, authz: authorizer, keyring: ring}
}

func (s *Service) List(ctx context.Context, userID, projectID uuid.UUID) ([]Registry, error) {
	if _, err := s.authz.Require(ctx, userID, projectID, authz.ViewProject); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListRegistryConnections(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list registries: %w", err)
	}
	registries := make([]Registry, len(rows))
	for i, row := range rows {
		registries[i] = toRegistry(row)
	}
	return registries, nil
}

type CreateInput struct {
	Name     string
	Host     string
	Username string
	Password string
}

// Create stores a registry with its password encrypted; credentials are checked by the Runner's first push
func (s *Service) Create(ctx context.Context, userID, projectID uuid.UUID, in CreateInput) (Registry, error) {
	if _, err := s.authz.Require(ctx, userID, projectID, authz.ManageApps); err != nil {
		return Registry{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Host = normalizeHost(in.Host)
	in.Username = strings.TrimSpace(in.Username)
	if err := checkInput(in); err != nil {
		return Registry{}, err
	}

	id := uuid.New()
	ciphertext, keyID := s.keyring.Seal([]byte(in.Password), passwordAAD(id))
	row, err := s.queries.CreateRegistryConnection(ctx, store.CreateRegistryConnectionParams{
		ID: id, ProjectID: projectID, Name: in.Name, Host: in.Host, Username: in.Username,
		PasswordCiphertext: ciphertext, PasswordKeyID: keyID, CreatedBy: &userID,
	})
	if database.IsUniqueViolation(err) {
		return Registry{}, errNameTaken
	}
	if err != nil {
		return Registry{}, fmt.Errorf("create registry: %w", err)
	}
	return toRegistry(row), nil
}

func (s *Service) Delete(ctx context.Context, userID, registryID uuid.UUID) error {
	if _, err := s.authorize(ctx, userID, registryID, authz.ManageApps); err != nil {
		return err
	}
	err := s.queries.DeleteRegistryConnection(ctx, registryID)
	if database.IsForeignKeyViolation(err) {
		return apperr.Conflict("registry_in_use", "Applications still push to this registry. Change or remove them first.")
	}
	if err != nil {
		return fmt.Errorf("delete registry: %w", err)
	}
	return nil
}

// CheckInProject reports errRegistryNotFound unless the registry belongs to projectID
func (s *Service) CheckInProject(ctx context.Context, projectID, registryID uuid.UUID) error {
	row, err := s.queries.GetRegistryConnection(ctx, registryID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && row.ProjectID != projectID) {
		return errRegistryNotFound
	}
	if err != nil {
		return fmt.Errorf("get registry: %w", err)
	}
	return nil
}

// Credentials decrypts push credentials; no user authorization, for handing a build to a Runner
func (s *Service) Credentials(ctx context.Context, registryID uuid.UUID) (Credentials, error) {
	row, err := s.queries.GetRegistryConnection(ctx, registryID)
	if err != nil {
		return Credentials{}, fmt.Errorf("get registry: %w", err)
	}
	password, err := s.keyring.Open(row.PasswordCiphertext, row.PasswordKeyID, passwordAAD(row.ID))
	if err != nil {
		return Credentials{}, fmt.Errorf("decrypt registry password: %w", err)
	}
	return Credentials{Host: row.Host, Username: row.Username, Password: string(password)}, nil
}

func (s *Service) authorize(ctx context.Context, userID, registryID uuid.UUID, p authz.Permission) (store.RegistryConnection, error) {
	row, err := s.queries.GetRegistryConnection(ctx, registryID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.RegistryConnection{}, errRegistryNotFound
	}
	if err != nil {
		return store.RegistryConnection{}, fmt.Errorf("get registry: %w", err)
	}
	if _, err := s.authz.Require(ctx, userID, row.ProjectID, p); err != nil {
		if errors.Is(err, authz.ErrProjectNotFound) {
			return store.RegistryConnection{}, errRegistryNotFound
		}
		return store.RegistryConnection{}, err
	}
	return row, nil
}

// normalizeHost turns "https://ghcr.io/" into "ghcr.io"
func normalizeHost(raw string) string {
	host := strings.ToLower(strings.TrimSpace(raw))
	host = strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://")
	return strings.TrimRight(host, "/")
}

func checkInput(in CreateInput) error {
	errs := validate.Errors{}
	if in.Name == "" || utf8.RuneCountInString(in.Name) > maxNameLength {
		errs.Add("name", fmt.Sprintf("Name must be 1–%d characters.", maxNameLength))
	}
	if !validHost.MatchString(in.Host) {
		errs.Add("host", "Use the registry host, such as ghcr.io, docker.io, or registry.example.com:5000.")
	}
	if in.Username == "" || len(in.Username) > maxUsernameLength || strings.ContainsAny(in.Username, ":\x00") {
		errs.Add("username", "Enter the registry username.")
	}
	if in.Password == "" || len(in.Password) > maxPasswordLength || strings.ContainsRune(in.Password, 0) {
		errs.Add("password", "Enter a password or access token with push permission.")
	}
	return errs.Err()
}

func passwordAAD(id uuid.UUID) []byte {
	return []byte("registry_connection:" + id.String() + ":password")
}

func toRegistry(row store.RegistryConnection) Registry {
	return Registry{
		ID: row.ID, ProjectID: row.ProjectID, Name: row.Name, Host: row.Host,
		Username: row.Username, CreatedAt: row.CreatedAt,
	}
}
