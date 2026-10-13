package httpx

import (
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/jakkayy/ForgeDeck/internal/ratelimit"
)

// ThrottleFailures stops a client address that keeps presenting bad
// credentials. It counts only 401 responses: a client that authenticates
// properly is never limited, which matters for a fleet of Agents behind one
// NAT address that all report every few seconds. Once an address has failed
// too often it gets 429 until its bucket refills.
func ThrottleFailures(limiter *ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := ClientIP(r)
			if blocked, retry := limiter.Blocked(key); blocked {
				WriteError(w, r, TooManyRequests(w, retry))
				return
			}
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			if ww.Status() == http.StatusUnauthorized {
				limiter.Fail(key)
			}
		})
	}
}
