package jobs

import (
	"context"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/store"
)

// JobKey is the Notifier key woken when a job's status or logs change
func JobKey(jobID uuid.UUID) string {
	return "job:" + jobID.String()
}

// notifyJob wakes watchers; inside a transaction it is delivered on commit
func notifyJob(ctx context.Context, q *store.Queries, jobID uuid.UUID) error {
	return q.NotifyWorkers(ctx, store.NotifyWorkersParams{Channel: NotifyChannel, Payload: JobKey(jobID)})
}
