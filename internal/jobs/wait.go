package jobs

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// recheckInterval bounds how long a missed notification can delay a claim.
const recheckInterval = 5 * time.Second

// ClaimForAgentWait claims a job for serverID, waiting up to maxWait for one
// to be queued. It returns (nil, nil) when the wait ends without a job or ctx
// is cancelled. stillAuthorized is re-checked on every wake-up so a waiting
// Agent whose server was removed is released promptly with its error.
func (s *Service) ClaimForAgentWait(
	ctx context.Context,
	notifier *Notifier,
	serverID uuid.UUID,
	workerID string,
	maxWait time.Duration,
	stillAuthorized func(context.Context) error,
) (*Claimed, error) {
	// Subscribe before the first claim so a job queued in between still wakes us.
	wake, unsubscribe := notifier.Subscribe(serverID.String())
	defer unsubscribe()

	deadline := time.NewTimer(maxWait)
	defer deadline.Stop()
	recheck := time.NewTicker(recheckInterval)
	defer recheck.Stop()

	for {
		job, err := s.ClaimForAgent(ctx, serverID, workerID)
		if err != nil || job != nil {
			return job, err
		}
		select {
		case <-ctx.Done():
			return nil, nil
		case <-deadline.C:
			return nil, nil
		case <-wake:
		case <-recheck.C:
		}
		if err := stillAuthorized(ctx); err != nil {
			return nil, err
		}
	}
}
