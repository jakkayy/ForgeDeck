package app_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/servers"
)

// Container output may be sensitive: lines stay with developers; that a job ran is visible to all.
func TestViewersCannotReadFetchedContainerLogs(t *testing.T) {
	w := newWorld(t)
	var queued servers.QueuedJob
	w.developer.Do(http.MethodPost, w.path("/api/v1/servers/{serverID}/containers/{containerID}/logs"), map[string]any{"tail_lines": 10}).
		RequireStatus(http.StatusAccepted).Decode(&queued)
	job := "/api/v1/jobs/" + queued.JobID.String()

	w.viewer.Do(http.MethodGet, job, nil).RequireStatus(http.StatusOK)
	for _, suffix := range []string{"/logs", "/events"} {
		if got := w.call(w.viewer, http.MethodGet, job+suffix, nil); got != http.StatusForbidden {
			t.Errorf("viewer reading %s: %d, want 403", suffix, got)
		}
		if got := w.call(w.developer, http.MethodGet, job+suffix, nil); got != http.StatusOK {
			t.Errorf("developer reading %s: %d, want 200", suffix, got)
		}
	}

	var jobID uuid.UUID
	if err := w.env.Pool.QueryRow(t.Context(), `SELECT id FROM jobs WHERE type = 'build'`).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	w.viewer.Do(http.MethodGet, "/api/v1/jobs/"+jobID.String()+"/logs", nil).RequireStatus(http.StatusOK)
}
