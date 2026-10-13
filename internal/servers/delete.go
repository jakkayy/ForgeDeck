package servers

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// Delete removes a server. Its Agent row goes with it, which revokes the
// Agent credential immediately: the next Agent request gets 401, and a
// pending long-poll claim is woken so it ends now rather than at its timeout.
// Containers already running on the host are left untouched.
func (s *Service) Delete(ctx context.Context, userID, serverID uuid.UUID) error {
	if _, err := s.authorizeServer(ctx, userID, serverID, authz.ManageServers); err != nil {
		return err
	}
	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		err := q.DeleteServer(ctx, serverID)
		if database.IsForeignKeyViolation(err) {
			return apperr.Conflict("server_in_use", "Environments still deploy to this server. Move or delete them first.")
		}
		if err != nil {
			return fmt.Errorf("delete server: %w", err)
		}
		return q.NotifyWorkers(ctx, store.NotifyWorkersParams{Channel: jobs.NotifyChannel, Payload: serverID.String()})
	})
}
