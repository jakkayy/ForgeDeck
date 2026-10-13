// Package jobs is the PostgreSQL-backed job queue: enqueueing, targeted
// claims with leases, progress events, and completion.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jakkayy/ForgeDeck/internal/authz"
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
	authz   *authz.Authorizer
	now     func() time.Time
}

// NewService returns a job Service; now is time.Now outside tests.
func NewService(pool *pgxpool.Pool, authorizer *authz.Authorizer, now func() time.Time) *Service {
	return &Service{pool: pool, queries: store.New(pool), authz: authorizer, now: now}
}

// NewJob describes a job to enqueue. TargetServerID is required for Agent
// jobs and must be nil for Runner jobs.
type NewJob struct {
	ProjectID      uuid.UUID
	ApplicationID  *uuid.UUID
	Type           string
	WorkerKind     WorkerKind
	TargetServerID *uuid.UUID
	Payload        any
	// MaxAttempts overrides the retry policy for the job type when > 0.
	MaxAttempts int32
	CreatedBy   *uuid.UUID
	// IdempotencyKey, when set, makes Enqueue return the existing job for the
	// same key in the project instead of queueing a duplicate.
	IdempotencyKey string
}

// Enqueue stores a queued job and notifies waiting workers when the
// transaction commits. With an IdempotencyKey already used in the project it
// returns that earlier job and created is false.
func (s *Service) Enqueue(ctx context.Context, job NewJob) (store.Job, error) {
	created, _, err := s.EnqueueOnce(ctx, job)
	return created, err
}

// EnqueueOnce is Enqueue that also reports whether a new job was created.
func (s *Service) EnqueueOnce(ctx context.Context, job NewJob) (store.Job, bool, error) {
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return store.Job{}, false, fmt.Errorf("encode job payload: %w", err)
	}
	var key *string
	if job.IdempotencyKey != "" {
		key = &job.IdempotencyKey
	}

	var stored store.Job
	isNew := true
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		stored, err = q.InsertJob(ctx, store.InsertJobParams{
			ProjectID:      job.ProjectID,
			ApplicationID:  job.ApplicationID,
			Type:           job.Type,
			WorkerKind:     string(job.WorkerKind),
			TargetServerID: job.TargetServerID,
			Payload:        payload,
			MaxAttempts:    maxAttempts(job.Type, job.MaxAttempts),
			IdempotencyKey: key,
			CreatedBy:      job.CreatedBy,
		})
		if errors.Is(err, pgx.ErrNoRows) && key != nil {
			isNew = false
			stored, err = q.GetJobByIdempotencyKey(ctx, store.GetJobByIdempotencyKeyParams{ProjectID: job.ProjectID, IdempotencyKey: key})
			return err
		}
		if err != nil {
			return fmt.Errorf("insert job: %w", err)
		}
		return q.NotifyWorkers(ctx, store.NotifyWorkersParams{Channel: NotifyChannel, Payload: notifyKey(job)})
	})
	return stored, isNew, err
}

func notifyKey(job NewJob) string {
	if job.WorkerKind == WorkerAgent && job.TargetServerID != nil {
		return job.TargetServerID.String()
	}
	return runnerNotifyKey
}
