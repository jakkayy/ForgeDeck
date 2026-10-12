// Package gitconn manages a project's connections to Git providers.
package gitconn

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/github"
	"github.com/jakkayy/ForgeDeck/internal/keyring"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// errConnectionNotFound also covers connections in projects the user cannot see.
var errConnectionNotFound = apperr.NotFound("Git connection not found.")

// Connection is a project's link to a Git provider. Credentials never leave
// the server.
type Connection struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	Provider    string    `json:"provider"`
	DisplayName string    `json:"display_name"`
	BaseURL     string    `json:"base_url"`
	CreatedAt   time.Time `json:"created_at"`
}

// Service implements Git connection operations.
type Service struct {
	pool    *pgxpool.Pool
	queries *store.Queries
	authz   *authz.Authorizer
	keyring *keyring.Keyring
	github  *github.Client // nil when the GitHub App is not configured
	now     func() time.Time
}

// NewService returns a Service. githubClient may be nil.
func NewService(pool *pgxpool.Pool, authorizer *authz.Authorizer, ring *keyring.Keyring, githubClient *github.Client, now func() time.Time) *Service {
	return &Service{pool: pool, queries: store.New(pool), authz: authorizer, keyring: ring, github: githubClient, now: now}
}

// List returns the project's connections.
func (s *Service) List(ctx context.Context, userID, projectID uuid.UUID) ([]Connection, error) {
	if _, err := s.authz.Require(ctx, userID, projectID, authz.ViewProject); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListGitConnections(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list git connections: %w", err)
	}
	connections := make([]Connection, len(rows))
	for i, row := range rows {
		connections[i] = toConnection(row)
	}
	return connections, nil
}

// Delete removes a connection. Stored credentials go with it.
func (s *Service) Delete(ctx context.Context, userID, connectionID uuid.UUID) error {
	if _, err := s.authorize(ctx, userID, connectionID, authz.ManageGit); err != nil {
		return err
	}
	if err := s.queries.DeleteGitConnection(ctx, connectionID); err != nil {
		return fmt.Errorf("delete git connection: %w", err)
	}
	return nil
}

// authorize loads a connection and checks the user's permission in its project.
func (s *Service) authorize(ctx context.Context, userID, connectionID uuid.UUID, p authz.Permission) (store.GitConnection, error) {
	row, err := s.queries.GetGitConnection(ctx, connectionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.GitConnection{}, errConnectionNotFound
	}
	if err != nil {
		return store.GitConnection{}, fmt.Errorf("get git connection: %w", err)
	}
	if _, err := s.authz.Require(ctx, userID, row.ProjectID, p); err != nil {
		if errors.Is(err, authz.ErrProjectNotFound) {
			return store.GitConnection{}, errConnectionNotFound
		}
		return store.GitConnection{}, err
	}
	return row, nil
}

func toConnection(row store.GitConnection) Connection {
	return Connection{
		ID: row.ID, ProjectID: row.ProjectID, Provider: row.Provider,
		DisplayName: row.DisplayName, BaseURL: row.BaseUrl, CreatedAt: row.CreatedAt,
	}
}
