// Package secrets stores a project's named secret values, encrypted and
// write-only, for injection into services at deployment time.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

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
	// MinValueLength keeps secrets long enough to be masked in logs without
	// also masking ordinary words.
	MinValueLength = 6
	maxValueLength = 4096
)

var (
	errSecretNotFound = apperr.NotFound("Secret not found.")
	// NamePattern is the form of a secret name: an environment variable name.
	NamePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,63}$`)
)

// Secret is the API view of a secret. The value is never part of it.
type Secret struct {
	Name      string    `json:"name"`
	Version   int32     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Service implements secret operations.
type Service struct {
	queries *store.Queries
	authz   *authz.Authorizer
	keyring *keyring.Keyring
}

// NewService returns a secrets Service.
func NewService(queries *store.Queries, authorizer *authz.Authorizer, ring *keyring.Keyring) *Service {
	return &Service{queries: queries, authz: authorizer, keyring: ring}
}

// List returns the project's secrets by name, without their values.
func (s *Service) List(ctx context.Context, userID, projectID uuid.UUID) ([]Secret, error) {
	if _, err := s.authz.Require(ctx, userID, projectID, authz.ViewProject); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListProjectSecrets(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	out := make([]Secret, len(rows))
	for i, row := range rows {
		out[i] = toSecret(row)
	}
	return out, nil
}

// Set creates the secret or replaces its value, and reports which. A
// replaced value takes effect at each environment's next deployment.
func (s *Service) Set(ctx context.Context, userID, projectID uuid.UUID, name, value string) (Secret, bool, error) {
	if _, err := s.authz.Require(ctx, userID, projectID, authz.ManageApps); err != nil {
		return Secret{}, false, err
	}
	if err := checkInput(name, value); err != nil {
		return Secret{}, false, err
	}

	existing, err := s.queries.GetProjectSecretByName(ctx, store.GetProjectSecretByNameParams{ProjectID: projectID, Name: name})
	if err == nil {
		ciphertext, keyID := s.keyring.Seal([]byte(value), valueAAD(existing.ID))
		row, err := s.queries.ReplaceProjectSecret(ctx, store.ReplaceProjectSecretParams{ID: existing.ID, Ciphertext: ciphertext, KeyID: keyID})
		if err != nil {
			return Secret{}, false, fmt.Errorf("replace secret: %w", err)
		}
		return toSecret(row), false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Secret{}, false, fmt.Errorf("get secret: %w", err)
	}

	id := uuid.New()
	ciphertext, keyID := s.keyring.Seal([]byte(value), valueAAD(id))
	row, err := s.queries.CreateProjectSecret(ctx, store.CreateProjectSecretParams{
		ID: id, ProjectID: projectID, Name: name, Ciphertext: ciphertext, KeyID: keyID, CreatedBy: &userID,
	})
	if database.IsUniqueViolation(err) {
		return Secret{}, false, apperr.Conflict("secret_changed", "Another request set this secret at the same time. Try again.")
	}
	if err != nil {
		return Secret{}, false, fmt.Errorf("create secret: %w", err)
	}
	return toSecret(row), true, nil
}

// Delete removes a secret that no environment lists.
func (s *Service) Delete(ctx context.Context, userID, projectID uuid.UUID, name string) error {
	if _, err := s.authz.Require(ctx, userID, projectID, authz.ManageApps); err != nil {
		return err
	}
	row, err := s.queries.GetProjectSecretByName(ctx, store.GetProjectSecretByNameParams{ProjectID: projectID, Name: name})
	if errors.Is(err, pgx.ErrNoRows) {
		return errSecretNotFound
	}
	if err != nil {
		return fmt.Errorf("get secret: %w", err)
	}
	users, err := s.queries.EnvironmentsUsingSecret(ctx, store.EnvironmentsUsingSecretParams{ProjectID: projectID, SecretName: name})
	if err != nil {
		return fmt.Errorf("check secret use: %w", err)
	}
	if len(users) > 0 {
		return apperr.Conflict("secret_in_use", "Environments still use this secret: "+strings.Join(users, ", ")+". Remove it from them first.")
	}
	if err := s.queries.DeleteProjectSecret(ctx, row.ID); err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	return nil
}

// Missing returns which of names are not secrets of the project.
func (s *Service) Missing(ctx context.Context, projectID uuid.UUID, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	rows, err := s.queries.ListProjectSecretsByNames(ctx, store.ListProjectSecretsByNamesParams{ProjectID: projectID, Names: names})
	if err != nil {
		return nil, fmt.Errorf("check secrets: %w", err)
	}
	found := make(map[string]bool, len(rows))
	for _, row := range rows {
		found[row.Name] = true
	}
	var missing []string
	for _, name := range names {
		if !found[name] {
			missing = append(missing, name)
		}
	}
	return missing, nil
}

// Values decrypts the named secrets of a project, and reports names that do
// not exist. It does no user authorization: it is for the Control Plane's own
// use when handing a deployment to an Agent, and its result must never be
// stored or logged.
func (s *Service) Values(ctx context.Context, projectID uuid.UUID, names []string) (values map[string]string, missing []string, err error) {
	values = map[string]string{}
	if len(names) == 0 {
		return values, nil, nil
	}
	rows, err := s.queries.ListProjectSecretsByNames(ctx, store.ListProjectSecretsByNamesParams{ProjectID: projectID, Names: names})
	if err != nil {
		return nil, nil, fmt.Errorf("get secrets: %w", err)
	}
	for _, row := range rows {
		value, err := s.keyring.Open(row.Ciphertext, row.KeyID, valueAAD(row.ID))
		if err != nil {
			return nil, nil, fmt.Errorf("decrypt secret %s: %w", row.Name, err)
		}
		values[row.Name] = string(value)
	}
	for _, name := range names {
		if _, ok := values[name]; !ok {
			missing = append(missing, name)
		}
	}
	return values, missing, nil
}

// AllValues decrypts every secret of a project. It is used to mask the
// values in logs and has the same handling rules as Values.
func (s *Service) AllValues(ctx context.Context, projectID uuid.UUID) ([]string, error) {
	rows, err := s.queries.ListProjectSecrets(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		value, err := s.keyring.Open(row.Ciphertext, row.KeyID, valueAAD(row.ID))
		if err != nil {
			return nil, fmt.Errorf("decrypt secret %s: %w", row.Name, err)
		}
		values = append(values, string(value))
	}
	return values, nil
}

func checkInput(name, value string) error {
	errs := validate.Errors{}
	if !NamePattern.MatchString(name) {
		errs.Add("name", "Use capital letters, digits and underscores, starting with a letter or underscore, such as DATABASE_PASSWORD.")
	}
	if len(value) < MinValueLength || len(value) > maxValueLength || strings.ContainsRune(value, 0) {
		errs.Add("value", fmt.Sprintf("The value must be %d bytes to 4 KB.", MinValueLength))
	}
	return errs.Err()
}

// valueAAD binds a value's ciphertext to its row.
func valueAAD(id uuid.UUID) []byte {
	return []byte("project_secret:" + id.String() + ":value")
}

func toSecret(row store.ProjectSecret) Secret {
	return Secret{Name: row.Name, Version: row.Version, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
