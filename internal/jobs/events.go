package jobs

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

var validStreams = map[string]bool{
	agentproto.StreamStdout: true, agentproto.StreamStderr: true, agentproto.StreamSystem: true,
}

// AppendLogs stores log events for an attempt the worker owns; duplicates are skipped
func (s *Service) AppendLogs(ctx context.Context, lease Lease, events []agentproto.LogEvent) error {
	if err := validateEvents(events); err != nil {
		return err
	}
	if err := s.requireActive(ctx, lease); err != nil {
		return err
	}

	redact, err := s.logRedaction(ctx, lease.JobID)
	if err != nil {
		return err
	}
	params := store.AppendJobLogsParams{
		JobID:           lease.JobID,
		AttemptNumber:   lease.Attempt,
		SequenceNumbers: make([]int64, len(events)),
		Streams:         make([]string, len(events)),
		Contents:        make([]string, len(events)),
	}
	for i, e := range events {
		params.SequenceNumbers[i], params.Streams[i], params.Contents[i] = e.Sequence, e.Stream, redact(e.Message)
	}
	if _, err := s.queries.AppendJobLogs(ctx, params); err != nil {
		return fmt.Errorf("append job logs: %w", err)
	}
	return notifyJob(ctx, s.queries, lease.JobID)
}

// logRedaction fails closed: if secrets cannot be read, the batch is not stored unmasked
func (s *Service) logRedaction(ctx context.Context, jobID uuid.UUID) (func(string) string, error) {
	if s.redactor == nil {
		return func(message string) string { return message }, nil
	}
	job, err := s.queries.GetJob(ctx, jobID)
	if err != nil {
		return nil, fmt.Errorf("load job for log redaction: %w", err)
	}
	redact, err := s.redactor.For(ctx, job.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("prepare log redaction: %w", err)
	}
	return redact, nil
}

func (s *Service) requireActive(ctx context.Context, lease Lease) error {
	active, err := s.queries.IsAttemptActive(ctx, store.IsAttemptActiveParams{
		ID: lease.JobID, Attempt: lease.Attempt, WorkerKind: string(lease.WorkerKind),
		WorkerID: lease.WorkerID, ServerID: lease.ServerID,
	})
	if err != nil {
		return fmt.Errorf("check job ownership: %w", err)
	}
	if !active {
		return ErrLeaseLost
	}
	return nil
}

func validateEvents(events []agentproto.LogEvent) error {
	errs := validate.Errors{}
	if len(events) > agentproto.MaxEventsPerBatch {
		errs.Add("events", fmt.Sprintf("At most %d events per batch.", agentproto.MaxEventsPerBatch))
		return errs.Err()
	}
	for i, e := range events {
		field := fmt.Sprintf("events[%d]", i)
		if e.Sequence < 0 {
			errs.Add(field+".sequence", "Sequence must not be negative.")
		}
		if !validStreams[e.Stream] {
			errs.Add(field+".stream", "Stream must be stdout, stderr or system.")
		}
		if len(e.Message) > agentproto.MaxEventBytes {
			errs.Add(field+".message", "Message is too long; split it into several events.")
		}
	}
	return errs.Err()
}
