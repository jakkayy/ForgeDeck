package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// ErrLeaseLost means the job attempt is no longer owned by the caller: its
// lease expired and was recovered, it was cancelled, or it already finished.
// The worker must stop and discard the attempt.
var ErrLeaseLost = apperr.Conflict("lease_lost", "This job attempt is no longer assigned to this worker.")

// AgentLease identifies a job attempt held by an Agent.
type AgentLease struct {
	JobID    uuid.UUID
	Attempt  int32
	ServerID uuid.UUID
	WorkerID string
}

// RenewAgentLease extends the lease on an attempt the Agent still owns.
func (s *Service) RenewAgentLease(ctx context.Context, lease AgentLease) (time.Time, error) {
	expiresAt, err := s.queries.RenewJobLease(ctx, store.RenewJobLeaseParams{
		ID:             lease.JobID,
		Attempt:        lease.Attempt,
		WorkerID:       lease.WorkerID,
		ServerID:       lease.ServerID,
		LeaseExpiresAt: s.now().Add(LeaseDuration),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, ErrLeaseLost
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("renew lease: %w", err)
	}
	return *expiresAt, nil
}
