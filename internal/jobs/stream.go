package jobs

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

const (
	streamBatch     = 500
	streamKeepAlive = 15 * time.Second
	// streamRecheck covers a missed notification, as for worker waits.
	streamRecheck = 5 * time.Second
)

func (st Status) finished() bool {
	return st == StatusSucceeded || st == StatusFailed || st == StatusCancelled
}

// Stream writes a job's logs after `after` and its status changes as SSE
// until the job finishes and every log line has been sent, the client goes
// away, or shutdown begins. Log events carry "<attempt>.<sequence>" ids so a
// reconnecting EventSource resumes exactly where it stopped.
func (s *Service) Stream(ctx context.Context, userID, jobID uuid.UUID, after LogCursor, notifier *Notifier, sse *httpx.SSE) error {
	if _, err := s.authorizeJob(ctx, userID, jobID); err != nil {
		return err
	}
	// Subscribe before the first read so no change between read and wait is missed.
	wake, unsubscribe := notifier.Subscribe(JobKey(jobID))
	defer unsubscribe()
	keepAlive := time.NewTicker(streamKeepAlive)
	defer keepAlive.Stop()
	recheck := time.NewTicker(streamRecheck)
	defer recheck.Stop()

	var lastStatus Status
	for {
		lines, err := s.Logs(ctx, userID, jobID, after, streamBatch)
		if err != nil {
			return err
		}
		for _, line := range lines {
			after = LogCursor{Attempt: line.Attempt, Sequence: line.Sequence}
			if err := sse.Event("log", after.String(), line); err != nil {
				return err
			}
		}
		job, err := s.Get(ctx, userID, jobID)
		if err != nil {
			return err
		}
		if job.Status != lastStatus {
			lastStatus = job.Status
			if err := sse.Event("status", "", job); err != nil {
				return err
			}
		}
		if err := sse.Flush(); err != nil {
			return err
		}
		if len(lines) == streamBatch {
			continue // more backlog to send before waiting
		}
		if job.Status.finished() {
			if err := sse.Event("end", "", map[string]Status{"status": job.Status}); err != nil {
				return err
			}
			return sse.Flush()
		}

		select {
		case <-ctx.Done():
			return nil
		case <-wake:
		case <-recheck.C:
		case <-keepAlive.C:
			if err := sse.Comment("keepalive"); err != nil {
				return err
			}
			if err := sse.Flush(); err != nil {
				return err
			}
		}
	}
}
