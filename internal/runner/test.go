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

// runTests runs the application's test command inside the image just built,
// never on the build host itself. The container has no network, no Linux
// capabilities, and bounded CPU, memory, and processes. It is named so it
// can be removed even when the job is cancelled mid-run, which only kills
// the docker CLI.
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
