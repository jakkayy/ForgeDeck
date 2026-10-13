package jobs

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

var errJobFinished = apperr.Conflict("job_finished", "This job has already finished.")

// Cancel stops a queued job, or marks a running one cancelled so its worker
// is refused at its next lease renewal and abandons the attempt. Work the
// worker already did (a container already restarted) is not undone.
func (s *Service) Cancel(ctx context.Context, userID, jobID uuid.UUID) (Job, error) {
	job, err := s.authorizeJob(ctx, userID, jobID)
	if err != nil {
		return Job{}, err
	}
	if _, err := s.authz.Require(ctx, userID, job.ProjectID, authz.OperateContainer); err != nil {
		return Job{}, err
	}
	previous, err := s.queries.CancelJob(ctx, store.CancelJobParams{ID: jobID, Now: s.now()})
	if err != nil {
		return Job{}, fmt.Errorf("cancel job: %w", err)
	}
	if previous != string(StatusQueued) && previous != string(StatusRunning) {
		return Job{}, errJobFinished
	}
	if err := notifyJob(ctx, s.queries, jobID); err != nil {
		return Job{}, err
	}
	return s.Get(ctx, userID, jobID)
}
