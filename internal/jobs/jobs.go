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

type WorkerKind string

const (
	WorkerAgent  WorkerKind = "agent"
	WorkerRunner WorkerKind = "runner"
)

type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// NotifyChannel payload is the target server ID for Agent jobs, "runner" for Runner jobs
const NotifyChannel = "forgedeck_jobs"

const runnerNotifyKey = "runner"

// SuccessHook runs in the transaction recording success; a failing hook must isolate itself (savepoint)
type SuccessHook func(ctx context.Context, tx pgx.Tx, job store.Job) error

type Service struct {
	pool        *pgxpool.Pool
	queries     *store.Queries
	authz       *authz.Authorizer
	now         func() time.Time
	onSucceeded []SuccessHook
	redactor    LogRedactor
}

type LogRedactor interface {
	For(ctx context.Context, projectID uuid.UUID) (func(string) string, error)
}

// SetLogRedactor masks stored log lines so unmasked secrets never reach the database; call while wiring
func (s *Service) SetLogRedactor(r LogRedactor) {
	s.redactor = r
}

// OnSucceeded registers a success hook; call while wiring
func (s *Service) OnSucceeded(hook SuccessHook) {
	s.onSucceeded = append(s.onSucceeded, hook)
}

func NewService(pool *pgxpool.Pool, authorizer *authz.Authorizer, now func() time.Time) *Service {
	return &Service{pool: pool, queries: store.New(pool), authz: authorizer, now: now}
}

// NewJob describes a job to enqueue; TargetServerID is required for Agent jobs, nil for Runner jobs
type NewJob struct {
	ProjectID      uuid.UUID
	ApplicationID  *uuid.UUID
	Type           string
	WorkerKind     WorkerKind
	TargetServerID *uuid.UUID
	Payload        any
	MaxAttempts    int32
	CreatedBy      *uuid.UUID
	IdempotencyKey string
}

// Enqueue stores a queued job and wakes workers on commit; a reused IdempotencyKey returns the earlier job
func (s *Service) Enqueue(ctx context.Context, job NewJob) (store.Job, error) {
	created, _, err := s.EnqueueOnce(ctx, job)
	return created, err
}

func (s *Service) EnqueueOnce(ctx context.Context, job NewJob) (store.Job, bool, error) {
	var stored store.Job
	isNew := true
	err := database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		stored, isNew, err = s.enqueue(ctx, s.queries.WithTx(tx), job)
		return err
	})
	return stored, isNew, err
}

// EnqueueIn queues a job inside the caller's transaction; idempotency keys are not supported
func (s *Service) EnqueueIn(ctx context.Context, tx pgx.Tx, job NewJob) (store.Job, error) {
	if job.IdempotencyKey != "" {
		return store.Job{}, errors.New("EnqueueIn does not support idempotency keys")
	}
	stored, _, err := s.enqueue(ctx, s.queries.WithTx(tx), job)
	return stored, err
}

func (s *Service) enqueue(ctx context.Context, q *store.Queries, job NewJob) (store.Job, bool, error) {
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return store.Job{}, false, fmt.Errorf("encode job payload: %w", err)
	}
	var key *string
	if job.IdempotencyKey != "" {
		key = &job.IdempotencyKey
	}

	stored, err := q.InsertJob(ctx, store.InsertJobParams{
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
		existing, err := q.GetJobByIdempotencyKey(ctx, store.GetJobByIdempotencyKeyParams{ProjectID: job.ProjectID, IdempotencyKey: key})
		return existing, false, err
	}
	if err != nil {
		return store.Job{}, false, fmt.Errorf("insert job: %w", err)
	}
	if err := q.NotifyWorkers(ctx, store.NotifyWorkersParams{Channel: NotifyChannel, Payload: notifyKey(job)}); err != nil {
		return store.Job{}, false, err
	}
	return stored, true, nil
}

func notifyKey(job NewJob) string {
	if job.WorkerKind == WorkerAgent && job.TargetServerID != nil {
		return job.TargetServerID.String()
	}
	return runnerNotifyKey
}
