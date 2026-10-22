// Package ratelimit provides an in-memory per-key token bucket limiter for one process.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const idleEviction = 15 * time.Minute

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

func New(every time.Duration, burst int, now func() time.Time) *Limiter {
	return &Limiter{every: every, burst: burst, now: now, buckets: map[string]*bucket{}}
}

// Allow consumes a token for key, or returns false and how long to wait
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

// Blocked reports whether key is out of tokens without using one; for failure limits (Blocked, then Fail)
func (l *Limiter) Blocked(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	b, ok := l.buckets[key]
	if !ok {
		return false, 0
	}
	tokens := b.limiter.TokensAt(now)
	if tokens >= 1 {
		return false, 0
	}
	return true, time.Duration((1 - tokens) * float64(l.every))
}

// Fail uses a token of key and never refuses
func (l *Limiter) Fail(key string) {
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
	b.limiter.AllowN(now, 1)
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
