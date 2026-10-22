package runner

import (
	"github.com/jakkayy/ForgeDeck/internal/procexec"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

// newExecutor runs build commands with HOME and DOCKER_CONFIG in the workspace and no Runner env
func newExecutor(ws *workspace, log worker.Logger) *procexec.Executor {
	return procexec.New(procexec.BaseEnv(ws.home, ws.dockerConfig), log)
}
