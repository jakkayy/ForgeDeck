package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// LeaseDuration is how long a claimed job stays assigned without a lease
// renewal. Short leases let a lost worker's job be recovered quickly.
const LeaseDuration = 60 * time.Second

// Claimed is a job assigned to a worker.
type Claimed struct {
	ID             uuid.UUID       `json:"id"`
	Type           string          `json:"type"`
	Attempt        int32           `json:"attempt"`
	Payload        json.RawMessage `json:"payload"`
	LeaseExpiresAt time.Time       `json:"lease_expires_at"`
}

// ClaimForAgent assigns the oldest queued job targeted at serverID to
// workerID. It returns (nil, nil) when nothing is queued. serverID must come
// from the authenticated Agent identity, never from the request.
func (s *Service) ClaimForAgent(ctx context.Context, serverID uuid.UUID, workerID string) (*Claimed, error) {
	now := s.now()
	var claimed *Claimed
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		job, err := q.ClaimAgentJob(ctx, store.ClaimAgentJobParams{
			ServerID:       serverID,
			WorkerID:       workerID,
			LeaseExpiresAt: now.Add(LeaseDuration),
			Now:            now,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("claim job: %w", err)
		}
		err = q.InsertJobAttempt(ctx, store.InsertJobAttemptParams{
			JobID: job.ID, AttemptNumber: job.AttemptCount, WorkerID: workerID, StartedAt: now,
		})
		if err != nil {
			return fmt.Errorf("record attempt: %w", err)
		}
		if err := notifyJob(ctx, q, job.ID); err != nil {
			return err
		}
		claimed = &Claimed{
			ID: job.ID, Type: job.Type, Attempt: job.AttemptCount,
			Payload: job.Payload, LeaseExpiresAt: *job.LeaseExpiresAt,
		}
		return nil
	})
	return claimed, err
}
