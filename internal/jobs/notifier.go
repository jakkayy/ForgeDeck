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

// Notifier holds one LISTEN connection and wakes waiters; a wake-up is a hint, waiters also recheck on a timer
type Notifier struct {
	pool   *pgxpool.Pool
	logger *slog.Logger

	mu      sync.Mutex
	waiters map[string]map[chan struct{}]struct{}
}

func NewNotifier(pool *pgxpool.Pool, logger *slog.Logger) *Notifier {
	return &Notifier{pool: pool, logger: logger, waiters: map[string]map[chan struct{}]struct{}{}}
}

// Subscribe returns a channel signalled when a job for key may be available; call unsubscribe when done
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

// Run listens until ctx is cancelled, reconnecting with backoff and waking all waiters after a reconnect
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
	// hold the connection out of the pool while listening
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
