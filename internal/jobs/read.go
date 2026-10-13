package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// errJobNotFound is also returned for jobs in projects the user does not
// belong to.
var errJobNotFound = apperr.NotFound("Job not found.")

// Job is the user-facing view of a job.
type Job struct {
	ID             uuid.UUID       `json:"id"`
	ProjectID      uuid.UUID       `json:"project_id"`
	Type           string          `json:"type"`
	Status         Status          `json:"status"`
	TargetServerID *uuid.UUID      `json:"target_server_id"`
	Payload        json.RawMessage `json:"payload"`
	Result         json.RawMessage `json:"result,omitempty"`
	Error          *JobError       `json:"error"`
	AttemptCount   int32           `json:"attempt_count"`
	MaxAttempts    int32           `json:"max_attempts"`
	CreatedAt      time.Time       `json:"created_at"`
	StartedAt      *time.Time      `json:"started_at"`
	FinishedAt     *time.Time      `json:"finished_at"`
	// LogsExpired says the job's output was removed by the retention job.
	LogsExpired bool `json:"logs_expired"`
}

// JobError explains why a job failed.
type JobError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// LogLine is one stored line of job output.
type LogLine struct {
	Attempt   int32     `json:"attempt"`
	Sequence  int64     `json:"sequence"`
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

// Get returns a job in a project the user belongs to.
func (s *Service) Get(ctx context.Context, userID, jobID uuid.UUID) (Job, error) {
	job, err := s.authorizeJob(ctx, userID, jobID)
	if err != nil {
		return Job{}, err
	}
	return toJob(job), nil
}

// LogCursor is the position after which logs are listed: "<attempt>.<sequence>".
type LogCursor struct {
	Attempt  int32
	Sequence int64
}

// ParseLogCursor reads a cursor; an empty string starts from the beginning.
func ParseLogCursor(raw string) (LogCursor, bool) {
	if raw == "" {
		return LogCursor{Attempt: 0, Sequence: -1}, true
	}
	attempt, sequence, found := strings.Cut(raw, ".")
	a, errA := strconv.ParseInt(attempt, 10, 32)
	q, errQ := strconv.ParseInt(sequence, 10, 64)
	if !found || errA != nil || errQ != nil {
		return LogCursor{}, false
	}
	return LogCursor{Attempt: int32(a), Sequence: q}, true
}

func (c LogCursor) String() string { return fmt.Sprintf("%d.%d", c.Attempt, c.Sequence) }

// Logs returns up to limit log lines after cursor, in order.
func (s *Service) Logs(ctx context.Context, userID, jobID uuid.UUID, after LogCursor, limit int32) ([]LogLine, error) {
	if err := s.authorizeJobOutput(ctx, userID, jobID); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListJobLogs(ctx, store.ListJobLogsParams{
		JobID: jobID, AfterAttempt: after.Attempt, AfterSequence: after.Sequence, PageLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list job logs: %w", err)
	}
	lines := make([]LogLine, len(rows))
	for i, r := range rows {
		lines[i] = LogLine{Attempt: r.AttemptNumber, Sequence: r.SequenceNumber, Stream: r.Stream, Message: r.Content, CreatedAt: r.CreatedAt}
	}
	return lines, nil
}

// authorizeJobOutput is authorizeJob for reading a job's log lines. The lines
// of a fetch_logs job are a container's output, which can hold sensitive
// data, so reading them takes the same role as fetching them; the output of
// builds and deployments is for every member.
func (s *Service) authorizeJobOutput(ctx context.Context, userID, jobID uuid.UUID) error {
	job, err := s.authorizeJob(ctx, userID, jobID)
	if err != nil {
		return err
	}
	if job.Type == agentproto.JobFetchLogs {
		_, err = s.authz.Require(ctx, userID, job.ProjectID, authz.ViewLogs)
	}
	return err
}

func (s *Service) authorizeJob(ctx context.Context, userID, jobID uuid.UUID) (store.Job, error) {
	job, err := s.queries.GetJob(ctx, jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Job{}, errJobNotFound
	}
	if err != nil {
		return store.Job{}, fmt.Errorf("get job: %w", err)
	}
	if _, err := s.authz.Require(ctx, userID, job.ProjectID, authz.ViewProject); err != nil {
		if errors.Is(err, authz.ErrProjectNotFound) {
			return store.Job{}, errJobNotFound
		}
		return store.Job{}, err
	}
	return job, nil
}

func toJob(j store.Job) Job {
	job := Job{
		ID: j.ID, ProjectID: j.ProjectID, Type: j.Type, Status: Status(j.Status),
		TargetServerID: j.TargetServerID, Payload: j.Payload, Result: j.Result,
		AttemptCount: j.AttemptCount, MaxAttempts: j.MaxAttempts,
		CreatedAt: j.CreatedAt, StartedAt: j.StartedAt, FinishedAt: j.FinishedAt, LogsExpired: j.LogsExpiredAt != nil,
	}
	if j.ErrorCode != nil {
		job.Error = &JobError{Code: *j.ErrorCode}
		if j.ErrorMessage != nil {
			job.Error.Message = *j.ErrorMessage
		}
	}
	return job
}
