package runner

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/dockertest"
)

func TestRunTestsIsolatesAndReportsFailure(t *testing.T) {
	dockertest.Require(t)
	ws, _ := newWorkspace(t.TempDir())
	t.Cleanup(func() { _ = ws.remove() })
	image := "busybox:1.37"

	cases := map[string]struct {
		command string
		ok      bool
	}{
		"passing tests":    {"echo ok && test -d /proc", true},
		"failing tests":    {"exit 3", false},
		"no network":       {"wget -q -T 2 -O /dev/null http://example.com", false},
		"pids are limited": {"for i in $(seq 100); do sleep 5 & done; wait", false},
	}
	limits := TestLimits{CPUs: "1", Memory: "64m", PIDs: 50}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			log := &recordLog{}
			err := runTests(context.Background(), newExecutor(ws, log), image, "forgedeck-test-"+strings.ReplaceAll(name, " ", "-"), tc.command, limits)
			if (err == nil) != tc.ok {
				t.Fatalf("err = %v, want ok=%v\n%s", err, tc.ok, log)
			}
		})
	}
}

func TestRunTestsRemovesContainerWhenCancelled(t *testing.T) {
	dockertest.Require(t)
	ws, _ := newWorkspace(t.TempDir())
	t.Cleanup(func() { _ = ws.remove() })
	name := "forgedeck-test-cancelled"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := runTests(ctx, newExecutor(ws, &recordLog{}), "busybox:1.37", name, "sleep 60", DefaultTestLimits); err == nil {
		t.Fatal("cancelled tests reported success")
	}
	out, _ := exec.CommandContext(context.Background(), "docker", "ps", "--all", "--quiet", "--filter", "name=^"+name+"$").Output()
	if len(strings.TrimSpace(string(out))) != 0 {
		t.Fatalf("container %s still exists", name)
	}
}
