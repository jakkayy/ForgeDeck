package jobs

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

const maxErrorMessageBytes = 2000

var validErrorCode = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// CompleteAgentJob records the outcome of an attempt the Agent owns. If the
// same outcome was already recorded for this attempt, it succeeds again so
// Agents can safely retry after a network failure.
func (s *Service) CompleteAgentJob(ctx context.Context, lease AgentLease, req agentproto.CompleteRequest) error {
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
		updated, err := q.CompleteAgentJob(ctx, store.CompleteAgentJobParams{
			ID: lease.JobID, Attempt: lease.Attempt, WorkerID: lease.WorkerID, ServerID: lease.ServerID,
			Status: req.Outcome, Result: result, Now: now,
			ErrorCode: optional(req.ErrorCode), ErrorMessage: optional(req.ErrorMessage),
		})
		if err != nil {
			return fmt.Errorf("complete job: %w", err)
		}
		if updated == 0 {
			return s.checkAlreadyCompleted(ctx, q, lease, req.Outcome)
		}
		return q.FinishJobAttempt(ctx, store.FinishJobAttemptParams{
			JobID: lease.JobID, AttemptNumber: lease.Attempt, Status: req.Outcome, Now: now,
			ErrorCode: optional(req.ErrorCode), ErrorSummary: optional(req.ErrorMessage),
		})
	})
}

func (s *Service) checkAlreadyCompleted(ctx context.Context, q *store.Queries, lease AgentLease, outcome string) error {
	status, err := q.GetAgentAttemptOutcome(ctx, store.GetAgentAttemptOutcomeParams{
		ID: lease.JobID, Attempt: lease.Attempt, WorkerID: lease.WorkerID, ServerID: lease.ServerID,
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
