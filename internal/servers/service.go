// Package servers manages Docker servers, their Agents, and Agent enrollment.
package servers

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/securetoken"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

const (
	enrollmentTokenTTL    = time.Hour
	enrollmentTokenPrefix = "fde_"
	maxServerNameLength   = 100
)

// Service implements server and Agent operations.
type Service struct {
	pool    *pgxpool.Pool
	queries *store.Queries
	authz   *authz.Authorizer
	now     func() time.Time
}

// NewService returns a server Service; now is time.Now outside tests.
func NewService(pool *pgxpool.Pool, authorizer *authz.Authorizer, now func() time.Time) *Service {
	return &Service{pool: pool, queries: store.New(pool), authz: authorizer, now: now}
}

// EnrollmentToken is a newly issued token. Token is shown once and only its
// hash is stored.
type EnrollmentToken struct {
	Token      string    `json:"token"`
	ServerName string    `json:"server_name"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// CreateEnrollmentToken issues a single-use token that registers a server
// named serverName in projectID when an Agent enrolls with it.
func (s *Service) CreateEnrollmentToken(ctx context.Context, userID, projectID uuid.UUID, serverName string) (EnrollmentToken, error) {
	if _, err := s.authz.Require(ctx, userID, projectID, authz.ManageServers); err != nil {
		return EnrollmentToken{}, err
	}
	serverName = strings.TrimSpace(serverName)
	if serverName == "" || utf8.RuneCountInString(serverName) > maxServerNameLength {
		return EnrollmentToken{}, validate.Errors{"server_name": fmt.Sprintf("Server name must be 1–%d characters.", maxServerNameLength)}
	}

	token, hash := securetoken.New(enrollmentTokenPrefix)
	record, err := s.queries.CreateEnrollmentToken(ctx, store.CreateEnrollmentTokenParams{
		ProjectID:  projectID,
		ServerName: serverName,
		TokenHash:  hash,
		CreatedBy:  &userID,
		ExpiresAt:  s.now().Add(enrollmentTokenTTL),
	})
	if err != nil {
		return EnrollmentToken{}, fmt.Errorf("create enrollment token: %w", err)
	}
	return EnrollmentToken{Token: token, ServerName: record.ServerName, ExpiresAt: record.ExpiresAt}, nil
}
