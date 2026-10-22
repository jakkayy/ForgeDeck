// Package health serves the liveness/readiness endpoint.
package health

import (
	"context"
	"net/http"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

const checkTimeout = 2 * time.Second

// Check reports whether a dependency is usable.
type Check func(ctx context.Context) error

// Handler returns 200 when every check passes, else 503; errors are not exposed
func Handler(checks map[string]Check) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
		defer cancel()

		status, code := "ok", http.StatusOK
		results := make(map[string]string, len(checks))
		for name, check := range checks {
			if err := check(ctx); err != nil {
				results[name] = "failing"
				status, code = "unavailable", http.StatusServiceUnavailable
				continue
			}
			results[name] = "ok"
		}
		_ = httpx.WriteJSON(w, code, map[string]any{"status": status, "checks": results})
	}
}
