package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	minReconnectDelay = time.Second
	maxReconnectDelay = 30 * time.Second
)

// Notifier holds one LISTEN connection per process and wakes in-memory
// waiters when a job is queued for their key (a server ID or "runner").
// A wake-up is only a hint to re-run the claim query; waiters must also
// recheck on a timer in case a notification is missed.
type Notifier struct {
	pool   *pgxpool.Pool
	logger *slog.Logger

	mu      sync.Mutex
	waiters map[string]map[chan struct{}]struct{}
}

// NewNotifier returns a Notifier; call Run to start listening.
func NewNotifier(pool *pgxpool.Pool, logger *slog.Logger) *Notifier {
	return &Notifier{pool: pool, logger: logger, waiters: map[string]map[chan struct{}]struct{}{}}
}

// Subscribe registers interest in key. The returned channel receives a value
// (without blocking the Notifier) when a job for key may be available.
// Callers must call unsubscribe when done.
func (n *Notifier) Subscribe(key string) (wake <-chan struct{}, unsubscribe func()) {
	ch := make(chan struct{}, 1)
	n.mu.Lock()
	if n.waiters[key] == nil {
		n.waiters[key] = map[chan struct{}]struct{}{}
	}
	n.waiters[key][ch] = struct{}{}
	n.mu.Unlock()

	return ch, func() {
		n.mu.Lock()
		delete(n.waiters[key], ch)
		if len(n.waiters[key]) == 0 {
			delete(n.waiters, key)
		}
		n.mu.Unlock()
	}
}

// Run listens until ctx is cancelled, reconnecting with backoff. After a
// reconnect every waiter is woken, since notifications may have been lost.
func (n *Notifier) Run(ctx context.Context) {
	delay := minReconnectDelay
	for ctx.Err() == nil {
		err := n.listen(ctx)
		if ctx.Err() != nil {
			return
		}
		n.logger.Warn("job notifier disconnected", "error", err, "retry_in", delay.String())
		n.wakeAll()
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, maxReconnectDelay)
	}
}

func (n *Notifier) listen(ctx context.Context) error {
	pooled, err := n.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	// Take the connection out of the pool: it is held for as long as we listen.
	conn := pooled.Hijack()
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()

	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{NotifyChannel}.Sanitize()); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	n.wakeAll()
	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			return fmt.Errorf("wait for notification: %w", err)
		}
		n.wake(notification.Payload)
	}
}

func (n *Notifier) wake(key string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for ch := range n.waiters[key] {
		signal(ch)
	}
}

func (n *Notifier) wakeAll() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, chans := range n.waiters {
		for ch := range chans {
			signal(ch)
		}
	}
}

func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default: // a wake-up is already pending
	}
}
