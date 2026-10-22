package runner

import (
	"context"
	"strconv"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/procexec"
)

// TestLimits bound a test container.
type TestLimits struct {
	CPUs   string // docker --cpus, e.g. "2"
	Memory string // docker --memory, e.g. "2g"; swap is capped to the same value
	PIDs   int
}

// DefaultTestLimits suit a small build host.
var DefaultTestLimits = TestLimits{CPUs: "2", Memory: "2g", PIDs: 512}

const removeContainerTimeout = 30 * time.Second

// runTests runs tests in the built image, never on the host: no network, no capabilities, bounded resources
func runTests(ctx context.Context, exec *procexec.Executor, image, name, testCommand string, limits TestLimits) error {
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), removeContainerTimeout)
		defer cancel()
		_, _ = exec.Output(cleanup, procexec.Command{Name: "docker", Args: []string{"rm", "--force", name}})
	}()
	return exec.Run(ctx, procexec.Command{
		Name: "docker",
		Args: []string{
			"run", "--rm", "--name", name,
			"--network", "none",
			"--cpus", limits.CPUs,
			"--memory", limits.Memory, "--memory-swap", limits.Memory,
			"--pids-limit", strconv.Itoa(limits.PIDs),
			"--cap-drop", "ALL",
			"--security-opt", "no-new-privileges",
			"--entrypoint", "/bin/sh",
			image, "-c", testCommand,
		},
	})
}
