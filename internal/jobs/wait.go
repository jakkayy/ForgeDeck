package jobs

import (
	"context"
	"time"
)

// recheckInterval bounds how long a missed notification can delay a claim
const recheckInterval = 5 * time.Second

const RunnerKey = runnerNotifyKey

type ClaimFunc func(ctx context.Context) (*Claimed, error)

// ClaimWait claims with claim, waiting up to maxWait; stillAuthorized is rechecked on every wake-up
func ClaimWait(
	ctx context.Context,
	notifier *Notifier,
	key string,
	claim ClaimFunc,
	maxWait time.Duration,
	stillAuthorized func(context.Context) error,
) (*Claimed, error) {
	// subscribe before the first claim so a job queued in between still wakes us
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
