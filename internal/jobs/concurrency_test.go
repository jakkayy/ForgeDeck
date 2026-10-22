package jobs_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// TestConcurrentClaimsNeverShareAJob: racing workers must claim every job exactly once.
func TestConcurrentClaimsNeverShareAJob(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	const jobCount, workers = 50, 10

	queued := map[uuid.UUID]bool{}
	for range jobCount {
		job, err := f.svc.Enqueue(ctx, f.agentJob(f.serverID))
		if err != nil {
			t.Fatal(err)
		}
		queued[job.ID] = true
	}

	var (
		mu      sync.Mutex
		claimed = map[uuid.UUID]string{}
		wg      sync.WaitGroup
		errs    = make(chan error, workers)
	)
	for w := range workers {
		wg.Go(func() {
			worker := fmt.Sprintf("worker-%d", w)
			for {
				job, err := f.svc.ClaimForAgent(ctx, f.serverID, worker)
				if err != nil {
					errs <- err
					return
				}
				if job == nil {
					return
				}
				mu.Lock()
				if previous, dup := claimed[job.ID]; dup {
					errs <- fmt.Errorf("job %s claimed by %s and %s", job.ID, previous, worker)
				}
				claimed[job.ID] = worker
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}

	if len(claimed) != jobCount {
		t.Fatalf("claimed %d jobs, want %d", len(claimed), jobCount)
	}
	for id := range claimed {
		if !queued[id] {
			t.Errorf("claimed unknown job %s", id)
		}
	}
	var attempts int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM job_attempts").Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != jobCount {
		t.Fatalf("%d attempts recorded, want %d (one per job)", attempts, jobCount)
	}
}
