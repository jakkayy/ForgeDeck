package servers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/securetoken"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

const (
	// AgentCredentialMaxAge is how long an Agent keeps a credential before the Control Plane asks it to replace it
	AgentCredentialMaxAge = 90 * 24 * time.Hour
	// CredentialGrace is how long a replaced credential keeps working, to survive a lost response
	CredentialGrace = 15 * time.Minute
)

// RotateCredential issues a new credential; the old one works for CredentialGrace and may retry
func (s *Service) RotateCredential(ctx context.Context, identity AgentIdentity, presented string) (agentproto.RotateResponse, error) {
	credential, hash := securetoken.New(agentproto.CredentialPrefix)
	now := s.now()
	rows, err := s.queries.RotateAgentCredential(ctx, store.RotateAgentCredentialParams{
		ID: identity.AgentID, PresentedHash: securetoken.Hash(presented), NewHash: hash,
		GraceUntil: now.Add(CredentialGrace), Now: now,
	})
	if err != nil {
		return agentproto.RotateResponse{}, fmt.Errorf("rotate agent credential: %w", err)
	}
	if rows == 0 {
		return agentproto.RotateResponse{}, ErrInvalidAgentCredential
	}
	return agentproto.RotateResponse{ServerID: identity.ServerID, Credential: credential}, nil
}

// RequestRotation asks the Agent to replace its credential at its next heartbeat
func (s *Service) RequestRotation(ctx context.Context, userID, serverID uuid.UUID) error {
	if _, err := s.authorizeServer(ctx, userID, serverID, authz.ManageServers); err != nil {
		return err
	}
	if _, err := s.queries.RequestAgentRotation(ctx, store.RequestAgentRotationParams{ServerID: serverID, Now: s.now()}); err != nil {
		return fmt.Errorf("request rotation: %w", err)
	}
	return nil
}
