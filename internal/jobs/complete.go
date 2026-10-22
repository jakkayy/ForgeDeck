package jobs

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

const maxErrorMessageBytes = 2000

var validErrorCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// Complete records an attempt's outcome; repeating the same outcome succeeds so workers can retry
func (s *Service) Complete(ctx context.Context, lease Lease, req agentproto.CompleteRequest) error {
	if err := validateCompletion(req); err != nil {
		return err
	}
	now := s.now()
	var result []byte
	if len(req.Result) > 0 {
		result = req.Result
	}

	return database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		updated, err := q.CompleteJob(ctx, store.CompleteJobParams{
			ID: lease.JobID, Attempt: lease.Attempt, WorkerKind: string(lease.WorkerKind),
			WorkerID: lease.WorkerID, ServerID: lease.ServerID,
			Status: req.Outcome, Result: result, Now: now,
			ErrorCode: optional(req.ErrorCode), ErrorMessage: optional(req.ErrorMessage),
		})
		if err != nil {
			return fmt.Errorf("complete job: %w", err)
		}
		if updated == 0 {
			return s.checkAlreadyCompleted(ctx, q, lease, req.Outcome)
		}
		err = q.FinishJobAttempt(ctx, store.FinishJobAttemptParams{
			JobID: lease.JobID, AttemptNumber: lease.Attempt, Status: req.Outcome, Now: now,
			ErrorCode: optional(req.ErrorCode), ErrorSummary: optional(req.ErrorMessage),
		})
		if err != nil {
			return err
		}
		if req.Outcome == agentproto.OutcomeSucceeded && len(s.onSucceeded) > 0 {
			if err := s.runSuccessHooks(ctx, tx, q, lease.JobID); err != nil {
				return err
			}
		}
		return notifyJob(ctx, q, lease.JobID)
	})
}

func (s *Service) runSuccessHooks(ctx context.Context, tx pgx.Tx, q *store.Queries, jobID uuid.UUID) error {
	job, err := q.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load completed job: %w", err)
	}
	for _, hook := range s.onSucceeded {
		if err := hook(ctx, tx, job); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) checkAlreadyCompleted(ctx context.Context, q *store.Queries, lease Lease, outcome string) error {
	status, err := q.GetAttemptOutcome(ctx, store.GetAttemptOutcomeParams{
		ID: lease.JobID, Attempt: lease.Attempt, WorkerKind: string(lease.WorkerKind),
		WorkerID: lease.WorkerID, ServerID: lease.ServerID,
	})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && status != outcome) {
		return ErrLeaseLost
	}
	if err != nil {
		return fmt.Errorf("check job outcome: %w", err)
	}
	return nil
}

func validateCompletion(req agentproto.CompleteRequest) error {
	errs := validate.Errors{}
	switch req.Outcome {
	case agentproto.OutcomeSucceeded:
		if req.ErrorCode != "" {
			errs.Add("error_code", "A succeeded job has no error code.")
		}
	case agentproto.OutcomeFailed:
		if !validErrorCode.MatchString(req.ErrorCode) {
			errs.Add("error_code", "A failed job needs a snake_case error code.")
		}
	default:
		errs.Add("outcome", "Outcome must be succeeded or failed.")
	}
	if len(req.ErrorMessage) > maxErrorMessageBytes {
		errs.Add("error_message", "Error message is too long.")
	}
	if len(req.Result) > agentproto.MaxResultBytes {
		errs.Add("result", "Result is too large.")
	}
	return errs.Err()
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
