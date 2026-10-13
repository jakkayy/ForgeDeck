package worker_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/worker"
)

type recorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *recorder) Log(stream, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, stream+": "+message)
}

func (r *recorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.lines, "\n")
}

func TestRedactingLogMasksEverySecretOccurrence(t *testing.T) {
	out := &recorder{}
	log := worker.NewRedactingLog(out, []string{"hunter22", "hunter22-long"})
	log.Log("stdout", "a hunter22 b hunter22-long c hunter22")
	if got := out.String(); got != "stdout: a ******** b ******** c ********" {
		t.Fatalf("got %q (the longer secret must win over its prefix)", got)
	}
}

func TestRedactingLogIgnoresShortAndEmptySecrets(t *testing.T) {
	out := &recorder{}
	worker.NewRedactingLog(out, []string{"", "abc"}).Log("stdout", "abc def")
	if out.String() != "stdout: abc def" {
		t.Fatalf("got %q", out.String())
	}
}
