package webhooks

import (
	"context"
	"fmt"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/builds"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// queueBuilds enqueues one build per application for the pushed commit and
// describes the outcome. Builds are keyed by application and commit, so the
// same commit pushed twice (or redelivered) is built once.
func (h *Handler) queueBuilds(ctx context.Context, apps []store.Application, push Push) (string, error) {
	if len(apps) == 0 {
		return "ignored: no application builds branch " + push.Branch, nil
	}
	queued := 0
	for _, app := range apps {
		job := builds.NewJob(app, push.Branch, push.CommitSHA, push.CommitMessage, agentproto.TriggerPush)
		job.IdempotencyKey = "build:" + app.ID.String() + ":" + push.CommitSHA
		_, created, err := h.jobs.EnqueueOnce(ctx, job)
		if err != nil {
			return "", fmt.Errorf("queue build for %s: %w", app.Name, err)
		}
		if created {
			queued++
		}
	}
	return fmt.Sprintf("queued %d of %d builds for %s", queued, len(apps), push.CommitSHA[:12]), nil
}
