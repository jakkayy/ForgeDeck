package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

// logBatcher numbers job output lines and sends them in batches, so a
// chatty job does not make one request per line.
type logBatcher struct {
	ctx    context.Context
	client *Client
	job    *agentproto.Job
	logger *slog.Logger

	mu      sync.Mutex
	pending []agentproto.LogEvent
	next    int64

	flush chan struct{}
	done  chan struct{}
}

func newLogBatcher(ctx context.Context, client *Client, job *agentproto.Job, logger *slog.Logger) *logBatcher {
	b := &logBatcher{
		ctx: ctx, client: client, job: job, logger: logger,
		next: 1, flush: make(chan struct{}, 1), done: make(chan struct{}),
	}
	go b.loop()
	return b
}

// Log queues one line, truncated to the protocol's per-event limit.
func (b *logBatcher) Log(stream, message string) {
	if len(message) > agentproto.MaxEventBytes {
		message = message[:agentproto.MaxEventBytes]
	}
	b.mu.Lock()
	b.pending = append(b.pending, agentproto.LogEvent{Sequence: b.next, Stream: stream, Message: message})
	b.next++
	full := len(b.pending) >= logBatchLines
	b.mu.Unlock()
	if full {
		select {
		case b.flush <- struct{}{}:
		default:
		}
	}
}

// Close sends any remaining lines and stops the batcher.
func (b *logBatcher) Close() {
	close(b.flush)
	<-b.done
}

func (b *logBatcher) loop() {
	defer close(b.done)
	ticker := time.NewTicker(logBatchDelay)
	defer ticker.Stop()
	for {
		select {
		case _, open := <-b.flush:
			b.send()
			if !open {
				return
			}
		case <-ticker.C:
			b.send()
		}
	}
}

func (b *logBatcher) send() {
	b.mu.Lock()
	batch := b.pending
	b.pending = nil
	b.mu.Unlock()

	for len(batch) > 0 {
		n := min(len(batch), agentproto.MaxEventsPerBatch)
		if err := b.client.SendEvents(b.ctx, b.job, batch[:n]); err != nil {
			b.logger.Warn("sending job logs failed; dropping batch", "error", err, "lines", n)
		}
		batch = batch[n:]
	}
}
