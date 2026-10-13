package servers

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/securetoken"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// ErrInvalidAgentCredential means the credential is unknown or its server
// was removed.
var ErrInvalidAgentCredential = errors.New("invalid agent credential")

// AgentIdentity is the authenticated Agent and the server it is bound to.
// Every Agent request is authorized against these values, never against IDs
// in the request body.
type AgentIdentity struct {
	AgentID   uuid.UUID
	ServerID  uuid.UUID
	ProjectID uuid.UUID
	// RotationDue says the Agent should replace its credential: an admin
	// asked, it is older than AgentCredentialMaxAge, or it presented a
	// credential that was already replaced.
	RotationDue bool
}

// AuthenticateAgent resolves a bearer credential to its Agent.
func (s *Service) AuthenticateAgent(ctx context.Context, credential string) (AgentIdentity, error) {
	now := s.now()
	row, err := s.queries.GetAgentByCredentialHash(ctx, store.GetAgentByCredentialHashParams{Hash: securetoken.Hash(credential), Now: now})
	if errors.Is(err, pgx.ErrNoRows) {
		return AgentIdentity{}, ErrInvalidAgentCredential
	}
	if err != nil {
		return AgentIdentity{}, fmt.Errorf("load agent: %w", err)
	}
	return AgentIdentity{
		AgentID: row.ID, ServerID: row.ServerID, ProjectID: row.ProjectID,
		RotationDue: row.IsPrevious || row.RotationRequestedAt != nil || now.Sub(row.CredentialRotatedAt) > AgentCredentialMaxAge,
	}, nil
}
