package runner

import (
	"github.com/jakkayy/ForgeDeck/internal/procexec"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

// newExecutor runs a build's commands inside its workspace: HOME and
// DOCKER_CONFIG point into it, and nothing of the Runner's own environment
// (which holds its credential) is inherited.
func newExecutor(ws *workspace, log worker.Logger) *procexec.Executor {
	return procexec.New(procexec.BaseEnv(ws.home, ws.dockerConfig), log)
}
