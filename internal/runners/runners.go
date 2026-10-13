// Package runners manages build Runners: their credentials and liveness.
package runners

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/securetoken"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// CredentialPrefix marks Runner credentials in logs and secret scanners.
const CredentialPrefix = "fdr_"

var (
	// ErrInvalidCredential means the credential is unknown or was revoked.
	ErrInvalidCredential = errors.New("invalid runner credential")
	// ErrNotFound means no Runner has the given name.
	ErrNotFound = errors.New("runner not found")

	validName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

// Identity is an authenticated Runner.
type Identity struct {
	ID   uuid.UUID
	Name string
}

// Service implements Runner operations.
type Service struct {
	queries *store.Queries
	now     func() time.Time
}

// NewService returns a Runner Service.
func NewService(queries *store.Queries, now func() time.Time) *Service {
	return &Service{queries: queries, now: now}
}

// Create registers a Runner and returns its credential. Only the hash is
// stored, so the credential cannot be shown again.
func (s *Service) Create(ctx context.Context, name string) (Identity, string, error) {
	if !validName.MatchString(name) {
		return Identity{}, "", errors.New("runner name must be lowercase letters, digits and hyphens, e.g. build-01")
	}
	credential, hash := securetoken.New(CredentialPrefix)
	row, err := s.queries.CreateRunner(ctx, store.CreateRunnerParams{Name: name, CredentialHash: hash})
	if database.IsUniqueViolation(err) {
		return Identity{}, "", fmt.Errorf("a runner named %q already exists", name)
	}
	if err != nil {
		return Identity{}, "", fmt.Errorf("create runner: %w", err)
	}
	return Identity{ID: row.ID, Name: row.Name}, credential, nil
}

// Delete removes a Runner, revoking its credential immediately.
func (s *Service) Delete(ctx context.Context, name string) error {
	n, err := s.queries.DeleteRunnerByName(ctx, name)
	if err != nil {
		return fmt.Errorf("delete runner: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Authenticate resolves a bearer credential to its Runner.
func (s *Service) Authenticate(ctx context.Context, credential string) (Identity, error) {
	row, err := s.queries.GetRunnerByCredentialHash(ctx, securetoken.Hash(credential))
	if errors.Is(err, pgx.ErrNoRows) {
		return Identity{}, ErrInvalidCredential
	}
	if err != nil {
		return Identity{}, fmt.Errorf("load runner: %w", err)
	}
	return Identity{ID: row.ID, Name: row.Name}, nil
}

// RecordHeartbeat stores liveness on the Control Plane clock.
func (s *Service) RecordHeartbeat(ctx context.Context, id uuid.UUID, version string) error {
	if len(version) > 64 {
		version = version[:64]
	}
	return s.queries.RecordRunnerHeartbeat(ctx, store.RecordRunnerHeartbeatParams{ID: id, Now: s.now(), Version: &version})
}
