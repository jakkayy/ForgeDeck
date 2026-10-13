package runner

import "github.com/jakkayy/ForgeDeck/internal/worker"

// Handlers returns the job handlers a Runner serves.
func Handlers() map[string]worker.Handler {
	return map[string]worker.Handler{}
}
