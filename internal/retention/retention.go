// Package retention removes old logs, jobs, sessions, tokens, deliveries and audit events; never releases or deployments.
package retention

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/store"
)

const (
	batchSize = 2000
	// sessionGrace and tokenGrace keep rows a little past their expiry
	sessionGrace = 24 * time.Hour
	tokenGrace   = 7 * 24 * time.Hour
)

// Policy is how long each kind of data is kept, in days; zero keeps it forever
type Policy struct {
	JobLogDays  int
	JobDays     int
	WebhookDays int
	AuditDays   int
}

// DefaultPolicy keeps logs a month, jobs six months, webhook deliveries a month, audit events a year
var DefaultPolicy = Policy{JobLogDays: 30, JobDays: 180, WebhookDays: 30, AuditDays: 365}

// Validate rejects negative days, or jobs removed before their logs expire
func (p Policy) Validate() error {
	for name, days := range map[string]int{"job logs": p.JobLogDays, "jobs": p.JobDays, "webhook deliveries": p.WebhookDays, "audit events": p.AuditDays} {
		if days < 0 {
			return fmt.Errorf("retention for %s must not be negative", name)
		}
	}
	if p.JobDays > 0 && p.JobLogDays > p.JobDays {
		return fmt.Errorf("job logs (%d days) cannot be kept longer than the jobs themselves (%d days)", p.JobLogDays, p.JobDays)
	}
	return nil
}

type Result struct {
	JobsWithExpiredLogs int
	JobLogLines         int64
	Jobs                int64
	Sessions            int64
	EnrollmentTokens    int64
	WebhookEvents       int64
	AuditEvents         int64
}

type Service struct {
	queries *store.Queries
	policy  Policy
	now     func() time.Time
}

func NewService(queries *store.Queries, policy Policy, now func() time.Time) *Service {
	return &Service{queries: queries, policy: policy, now: now}
}

func days(n int) time.Duration { return time.Duration(n) * 24 * time.Hour }

// RunOnce applies the policy; it stops at the first error and is safe to repeat or run concurrently
func (s *Service) RunOnce(ctx context.Context) (Result, error) {
	now := s.now()
	var r Result
	var err error

	if s.policy.JobLogDays > 0 {
		if r.JobsWithExpiredLogs, r.JobLogLines, err = s.expireJobLogs(ctx, now.Add(-days(s.policy.JobLogDays)), now); err != nil {
			return r, err
		}
	}
	if s.policy.JobDays > 0 {
		cutoff := now.Add(-days(s.policy.JobDays))
		if r.Jobs, err = s.untilEmpty(func() (int64, error) {
			return s.queries.DeleteOldJobs(ctx, store.DeleteOldJobsParams{Cutoff: cutoff, Batch: batchSize})
		}); err != nil {
			return r, fmt.Errorf("delete old jobs: %w", err)
		}
	}
	if r.Sessions, err = s.untilEmpty(func() (int64, error) {
		return s.queries.DeleteExpiredSessions(ctx, store.DeleteExpiredSessionsParams{Cutoff: now.Add(-sessionGrace), Batch: batchSize})
	}); err != nil {
		return r, fmt.Errorf("delete expired sessions: %w", err)
	}
	if r.EnrollmentTokens, err = s.untilEmpty(func() (int64, error) {
		return s.queries.DeleteOldEnrollmentTokens(ctx, store.DeleteOldEnrollmentTokensParams{Cutoff: now.Add(-tokenGrace), Batch: batchSize})
	}); err != nil {
		return r, fmt.Errorf("delete old enrollment tokens: %w", err)
	}
	if s.policy.WebhookDays > 0 {
		cutoff := now.Add(-days(s.policy.WebhookDays))
		if r.WebhookEvents, err = s.untilEmpty(func() (int64, error) {
			return s.queries.DeleteOldWebhookEvents(ctx, store.DeleteOldWebhookEventsParams{Cutoff: cutoff, Batch: batchSize})
		}); err != nil {
			return r, fmt.Errorf("delete old webhook events: %w", err)
		}
	}
	if s.policy.AuditDays > 0 {
		cutoff := now.Add(-days(s.policy.AuditDays))
		if r.AuditEvents, err = s.untilEmpty(func() (int64, error) {
			return s.queries.DeleteOldAuditLogs(ctx, store.DeleteOldAuditLogsParams{Cutoff: cutoff, Batch: batchSize})
		}); err != nil {
			return r, fmt.Errorf("delete old audit events: %w", err)
		}
	}
	return r, nil
}

// expireJobLogs removes log lines of finished jobs older than cutoff, in batches
func (s *Service) expireJobLogs(ctx context.Context, cutoff, now time.Time) (jobs int, lines int64, err error) {
	for {
		ids, err := s.queries.MarkExpiredJobLogs(ctx, store.MarkExpiredJobLogsParams{Cutoff: cutoff, Now: now, Batch: batchSize})
		if err != nil {
			return jobs, lines, fmt.Errorf("mark jobs with expired logs: %w", err)
		}
		if len(ids) == 0 {
			return jobs, lines, nil
		}
		deleted, err := s.queries.DeleteJobLogLines(ctx, ids)
		if err != nil {
			return jobs, lines, fmt.Errorf("delete job log lines: %w", err)
		}
		jobs, lines = jobs+len(ids), lines+deleted
	}
}

func (s *Service) untilEmpty(deleteBatch func() (int64, error)) (int64, error) {
	var total int64
	for {
		n, err := deleteBatch()
		total += n
		if err != nil || n < batchSize {
			return total, err
		}
	}
}

// Run applies the policy shortly after start and then every interval until ctx is cancelled
func (s *Service) Run(ctx context.Context, logger *slog.Logger, interval time.Duration) {
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		result, err := s.RunOnce(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			logger.Error("retention cleanup failed", "error", err, "removed", result)
		case result != (Result{}):
			logger.Info("retention cleanup removed data", "removed", result)
		}
		timer.Reset(interval)
	}
}
