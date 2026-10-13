package runner

import (
	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

// Handlers returns the job handlers a Runner serves.
func Handlers(build BuildConfig) map[string]worker.Handler {
	return map[string]worker.Handler{
		agentproto.JobBuild: BuildHandler(build),
	}
}
