package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/store"
)

// recoveryInterval is how often expired leases are swept. With a 60-second
// lease, a lost job is retried within about 75 seconds.
const recoveryInterval = 15 * time.Second

// RecoverExpiredLeases requeues (or fails, when out of attempts) running jobs
// whose worker stopped renewing its lease, and wakes workers for requeued
// jobs. A lease expiring does not prove the old worker stopped, which is why
// job handlers must be idempotent.
func (s *Service) RecoverExpiredLeases(ctx context.Context) (requeued, failed int, err error) {
	rows, err := s.queries.RecoverExpiredLeases(ctx, s.now())
	if err != nil {
		return 0, 0, fmt.Errorf("recover expired leases: %w", err)
	}
	for _, row := range rows {
		if Status(row.Status) == StatusFailed {
			failed++
			continue
		}
		requeued++
		key := runnerNotifyKey
		if row.TargetServerID != nil {
			key = row.TargetServerID.String()
		}
		if err := s.queries.NotifyWorkers(ctx, store.NotifyWorkersParams{Channel: NotifyChannel, Payload: key}); err != nil {
			return requeued, failed, fmt.Errorf("notify workers: %w", err)
		}
	}
	return requeued, failed, nil
}

// RunRecovery sweeps expired leases until ctx is cancelled.
func (s *Service) RunRecovery(ctx context.Context, logger *slog.Logger) {
	ticker := time.NewTicker(recoveryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		requeued, failed, err := s.RecoverExpiredLeases(ctx)
		switch {
		case err != nil && ctx.Err() == nil:
			logger.Error("lease recovery failed", "error", err)
		case requeued+failed > 0:
			logger.Warn("recovered jobs with expired leases", "requeued", requeued, "failed", failed)
		}
	}
}
