package httpx

import (
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/jakkayy/ForgeDeck/internal/ratelimit"
)

// ThrottleFailures answers 429 to addresses that keep sending bad credentials; only 401s count
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
