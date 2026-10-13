package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

const (
	claimWait       = agentproto.MaxClaimWaitSeconds * time.Second
	leaseRenewal    = 20 * time.Second
	maxClaimBackoff = 30 * time.Second
	logBatchLines   = 100
	logBatchDelay   = time.Second
)

// JobError is a handler failure with a stable code shown to users.
type JobError struct {
	Code    string
	Message string
}

func (e *JobError) Error() string { return e.Code + ": " + e.Message }

// Logger receives a job's output lines. Lines are sent to the Control Plane
// in batches.
type Logger interface {
	Log(stream, message string)
}

// Handler runs one job type. A nil error with a non-nil result reports
// success with that result; errors are reported as failures.
type Handler func(ctx context.Context, job *agentproto.Job, log Logger) (result any, err error)

// Worker claims jobs and runs them one at a time.
type Worker struct {
	client   *Client
	handlers map[string]Handler
	logger   *slog.Logger
	// afterJob runs after every attempt, e.g. to refresh the container snapshot.
	afterJob func()
}

// NewWorker returns a worker that dispatches by job type to handlers.
func NewWorker(client *Client, handlers map[string]Handler, afterJob func(), logger *slog.Logger) *Worker {
	return &Worker{client: client, handlers: handlers, afterJob: afterJob, logger: logger}
}

// Run claims and executes jobs until ctx ends. It returns ErrUnauthorized
// when the credential is rejected; other errors back off and retry.
func (w *Worker) Run(ctx context.Context) error {
	backoff := time.Second
	for ctx.Err() == nil {
		job, err := w.client.Claim(ctx, claimWait)
		switch {
		case errors.Is(err, ErrUnauthorized):
			return err
		case err != nil:
			if ctx.Err() != nil {
				return nil
			}
			w.logger.Warn("job claim failed", "error", err, "retry_in", backoff.String())
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(Jitter(backoff)):
			}
			backoff = min(backoff*2, maxClaimBackoff)
			continue
		}
		backoff = time.Second
		if job != nil {
			w.execute(ctx, job)
		}
	}
	return nil
}

// execute runs one attempt while keeping its lease alive, then reports the
// outcome. If the lease is lost the handler's context is cancelled and the
// outcome is discarded: another attempt owns the job now.
func (w *Worker) execute(ctx context.Context, job *agentproto.Job) {
	logger := w.logger.With("job_id", job.ID, "job_type", job.Type, "attempt", job.Attempt)
	logger.Info("job started")
	jobCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	go w.keepLease(jobCtx, cancel, job)

	output := newLogBatcher(jobCtx, w.client, job, logger)
	result, runErr := w.run(jobCtx, job, output)
	output.Close()
	if w.afterJob != nil {
		w.afterJob()
	}

	if errors.Is(context.Cause(jobCtx), ErrLeaseLost) {
		logger.Warn("job lease lost; discarding outcome")
		return
	}
	if err := w.client.Complete(ctx, job, completion(result, runErr)); err != nil {
		logger.Error("reporting job outcome failed", "error", err)
		return
	}
	logger.Info("job finished", "failed", runErr != nil)
}

func (w *Worker) run(ctx context.Context, job *agentproto.Job, output Logger) (any, error) {
	handler, ok := w.handlers[job.Type]
	if !ok {
		return nil, &JobError{Code: "unsupported_job_type", Message: fmt.Sprintf("This worker cannot run %q jobs. Upgrade it.", job.Type)}
	}
	return handler(ctx, job, output)
}

func (w *Worker) keepLease(ctx context.Context, cancel context.CancelCauseFunc, job *agentproto.Job) {
	ticker := time.NewTicker(leaseRenewal)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.client.RenewLease(ctx, job); errors.Is(err, ErrLeaseLost) || errors.Is(err, ErrUnauthorized) {
				cancel(ErrLeaseLost)
				return
			} else if err != nil && ctx.Err() == nil {
				w.logger.Warn("lease renewal failed", "job_id", job.ID, "error", err)
			}
		}
	}
}

func completion(result any, err error) agentproto.CompleteRequest {
	if err != nil {
		var jobErr *JobError
		if !errors.As(err, &jobErr) {
			jobErr = &JobError{Code: "worker_error", Message: err.Error()}
		}
		return agentproto.CompleteRequest{Outcome: agentproto.OutcomeFailed, ErrorCode: jobErr.Code, ErrorMessage: jobErr.Message}
	}
	req := agentproto.CompleteRequest{Outcome: agentproto.OutcomeSucceeded}
	if result != nil {
		if encoded, err := json.Marshal(result); err == nil {
			req.Result = encoded
		}
	}
	return req
}
