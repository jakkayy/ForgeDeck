package webhooks_test

import (
	"net/http"
	"sync"
	"testing"
)

// TestConcurrentRedeliveriesBuildOnce sends the same delivery several times
// at once, as overlapping provider retries do, and expects exactly one
// recorded event and one build.
func TestConcurrentRedeliveriesBuildOnce(t *testing.T) {
	p := newGitLabProject(t)
	const commit = "da1560886d4f094c3e6c9ef40349f7d38b5d27d7"

	statuses := make(chan int, 5)
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() { statuses <- p.push(t, "same-delivery", "main", commit).Status })
	}
	wg.Wait()
	close(statuses)

	accepted := 0
	for status := range statuses {
		switch status {
		case http.StatusAccepted:
			accepted++
		case http.StatusOK: // reported as a duplicate
		default:
			t.Errorf("unexpected status %d", status)
		}
	}
	if accepted != 1 {
		t.Fatalf("%d deliveries processed, want 1", accepted)
	}
	var events int
	if err := p.env.Pool.QueryRow(t.Context(), "SELECT count(*) FROM webhook_events").Scan(&events); err != nil || events != 1 {
		t.Fatalf("%d events recorded (err %v), want 1", events, err)
	}
	if n := len(buildJobs(t, p.env)); n != 1 {
		t.Fatalf("%d builds, want 1", n)
	}
}
