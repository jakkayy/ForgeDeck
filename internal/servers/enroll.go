package servers

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/securetoken"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

var (
	// ErrInvalidEnrollmentToken covers unknown, expired and used tokens alike so the cause is not revealed
	ErrInvalidEnrollmentToken = errors.New("invalid enrollment token")

	errServerNameTaken = apperr.Conflict("server_name_taken",
		"A server with this name already exists in the project. Remove it or issue a token with another name.")
)

// Enroll consumes an enrollment token and registers a server and Agent in one transaction
func (s *Service) Enroll(ctx context.Context, token, agentVersion string) (agentproto.EnrollResponse, error) {
	credential, credentialHash := securetoken.New(agentproto.CredentialPrefix)
	var resp agentproto.EnrollResponse

	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		enrollment, err := q.ConsumeEnrollmentToken(ctx, store.ConsumeEnrollmentTokenParams{
			TokenHash: securetoken.Hash(token),
			Now:       s.now(),
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidEnrollmentToken
		}
		if err != nil {
			return fmt.Errorf("consume enrollment token: %w", err)
		}

		server, err := q.CreateServer(ctx, store.CreateServerParams{
			ProjectID:    enrollment.ProjectID,
			Name:         enrollment.ServerName,
			AgentVersion: &agentVersion,
		})
		if database.IsUniqueViolation(err) {
			return errServerNameTaken
		}
		if err != nil {
			return fmt.Errorf("create server: %w", err)
		}

		agent, err := q.CreateAgent(ctx, store.CreateAgentParams{ServerID: server.ID, CredentialHash: credentialHash})
		if err != nil {
			return fmt.Errorf("create agent: %w", err)
		}
		if err := q.SetEnrollmentTokenServer(ctx, store.SetEnrollmentTokenServerParams{ID: enrollment.ID, ServerID: &server.ID}); err != nil {
			return fmt.Errorf("link enrollment token: %w", err)
		}

		resp = agentproto.EnrollResponse{AgentID: agent.ID, ServerID: server.ID, Credential: credential}
		return nil
	})
	return resp, err
}
