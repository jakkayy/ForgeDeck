package jobs

import (
	"context"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/store"
)

// JobKey is the Notifier key woken when a job's status or logs change. It
// shares the workers' LISTEN channel, so live updates need no extra connection.
func JobKey(jobID uuid.UUID) string {
	return "job:" + jobID.String()
}

// notifyJob wakes anyone watching the job. Inside a transaction the
// notification is delivered on commit, so watchers always see the change.
func notifyJob(ctx context.Context, q *store.Queries, jobID uuid.UUID) error {
	return q.NotifyWorkers(ctx, store.NotifyWorkersParams{Channel: NotifyChannel, Payload: JobKey(jobID)})
}
