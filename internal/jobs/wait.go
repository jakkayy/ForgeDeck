package jobs

import (
	"context"
	"time"
)

// recheckInterval bounds how long a missed notification can delay a claim.
const recheckInterval = 5 * time.Second

// RunnerKey is the Notifier key woken when a Runner job is queued.
const RunnerKey = runnerNotifyKey

// ClaimFunc tries to claim one job; (nil, nil) means nothing is queued.
type ClaimFunc func(ctx context.Context) (*Claimed, error)

// ClaimWait claims with claim, waiting up to maxWait for a job when none is
// queued. key is what wakes the wait: an Agent's server ID or RunnerKey. It
// returns (nil, nil) when the wait ends without a job or ctx is cancelled.
// stillAuthorized is re-checked on every wake-up so a worker revoked while
// waiting is released promptly with its error.
func ClaimWait(
	ctx context.Context,
	notifier *Notifier,
	key string,
	claim ClaimFunc,
	maxWait time.Duration,
	stillAuthorized func(context.Context) error,
) (*Claimed, error) {
	// Subscribe before the first claim so a job queued in between still wakes us.
	wake, unsubscribe := notifier.Subscribe(key)
	defer unsubscribe()

	deadline := time.NewTimer(maxWait)
	defer deadline.Stop()
	recheck := time.NewTicker(recheckInterval)
	defer recheck.Stop()

	for {
		job, err := claim(ctx)
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
