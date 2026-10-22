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

// ErrLeaseLost means the attempt is no longer owned by the caller; the worker must discard it
var ErrLeaseLost = apperr.Conflict("lease_lost", "This job attempt is no longer assigned to this worker.")

// Lease identifies a job attempt held by a worker; ownership checks match all fields
type Lease struct {
	JobID      uuid.UUID
	Attempt    int32
	WorkerKind WorkerKind
	WorkerID   string
	ServerID   *uuid.UUID
}

func (s *Service) RenewLease(ctx context.Context, lease Lease) (time.Time, error) {
	expiresAt, err := s.queries.RenewJobLease(ctx, store.RenewJobLeaseParams{
		ID:             lease.JobID,
		Attempt:        lease.Attempt,
		WorkerKind:     string(lease.WorkerKind),
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
