package runner

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

const dockerStatusTimeout = 10 * time.Second

// DockerStatus reports whether the docker CLI can reach its daemon. Builds
// run through the CLI, so this is the check that matters.
func DockerStatus(ctx context.Context) (agentproto.Docker, *agentproto.Resources) {
	ctx, cancel := context.WithTimeout(ctx, dockerStatusTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}").Output()
	if err != nil {
		return agentproto.Docker{Available: false}, nil
	}
	return agentproto.Docker{Available: true, Version: strings.TrimSpace(string(out))}, nil
}
