package servers

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/authz"
)

// Delete removes a server. Its Agent row goes with it, which revokes the
// Agent credential immediately: the next Agent request gets 401. Containers
// already running on the host are left untouched.
func (s *Service) Delete(ctx context.Context, userID, serverID uuid.UUID) error {
	if _, err := s.authorizeServer(ctx, userID, serverID, authz.ManageServers); err != nil {
		return err
	}
	if err := s.queries.DeleteServer(ctx, serverID); err != nil {
		return fmt.Errorf("delete server: %w", err)
	}
	return nil
}
