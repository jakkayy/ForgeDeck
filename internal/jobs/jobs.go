// Package jobs is the PostgreSQL-backed job queue: enqueueing, targeted
// claims with leases, progress events, and completion.
package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// WorkerKind says which kind of worker may claim a job.
type WorkerKind string

const (
	WorkerAgent  WorkerKind = "agent"
	WorkerRunner WorkerKind = "runner"
)

// Status is a job's lifecycle state.
type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// NotifyChannel carries one notification per queued job. The payload is the
// target server ID for Agent jobs and "runner" for Runner jobs.
const NotifyChannel = "forgedeck_jobs"

const runnerNotifyKey = "runner"

// Service implements the job queue.
type Service struct {
	pool    *pgxpool.Pool
	queries *store.Queries
	now     func() time.Time
}

// NewService returns a job Service; now is time.Now outside tests.
func NewService(pool *pgxpool.Pool, now func() time.Time) *Service {
	return &Service{pool: pool, queries: store.New(pool), now: now}
}

// NewJob describes a job to enqueue. TargetServerID is required for Agent
// jobs and must be nil for Runner jobs.
type NewJob struct {
	ProjectID      uuid.UUID
	Type           string
	WorkerKind     WorkerKind
	TargetServerID *uuid.UUID
	Payload        any
	MaxAttempts    int32
	CreatedBy      *uuid.UUID
}

// Enqueue stores a queued job and notifies waiting workers when the
// transaction commits.
func (s *Service) Enqueue(ctx context.Context, job NewJob) (store.Job, error) {
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return store.Job{}, fmt.Errorf("encode job payload: %w", err)
	}
	maxAttempts := max(job.MaxAttempts, 1)

	var created store.Job
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		created, err = q.InsertJob(ctx, store.InsertJobParams{
			ProjectID:      job.ProjectID,
			Type:           job.Type,
			WorkerKind:     string(job.WorkerKind),
			TargetServerID: job.TargetServerID,
			Payload:        payload,
			MaxAttempts:    maxAttempts,
			CreatedBy:      job.CreatedBy,
		})
		if err != nil {
			return fmt.Errorf("insert job: %w", err)
		}
		return q.NotifyJobQueued(ctx, store.NotifyJobQueuedParams{Channel: NotifyChannel, Payload: notifyKey(job)})
	})
	return created, err
}

func notifyKey(job NewJob) string {
	if job.WorkerKind == WorkerAgent && job.TargetServerID != nil {
		return job.TargetServerID.String()
	}
	return runnerNotifyKey
}
