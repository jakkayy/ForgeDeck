// Package ratelimit provides an in-memory, per-key token bucket limiter for
// a single Control Plane process.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// idleEviction removes keys that have been quiet long enough to be full again.
const idleEviction = 15 * time.Minute

// Limiter tracks a token bucket per key (an IP address, an email, ...).
type Limiter struct {
	every time.Duration
	burst int
	now   func() time.Time

	mu        sync.Mutex
	buckets   map[string]*bucket
	lastSweep time.Time
}

type bucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// New allows burst events at once, then one event per every.
func New(every time.Duration, burst int, now func() time.Time) *Limiter {
	return &Limiter{every: every, burst: burst, now: now, buckets: map[string]*bucket{}}
}

// Allow consumes a token for key. When none is left it returns false and how
// long the caller should wait before retrying.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweep(now)

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{limiter: rate.NewLimiter(rate.Every(l.every), l.burst)}
		l.buckets[key] = b
	}
	b.lastSeen = now

	reservation := b.limiter.ReserveN(now, 1)
	if delay := reservation.DelayFrom(now); delay > 0 {
		reservation.CancelAt(now)
		return false, delay
	}
	return true, 0
}

func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	for key, b := range l.buckets {
		if now.Sub(b.lastSeen) > idleEviction {
			delete(l.buckets, key)
		}
	}
}
