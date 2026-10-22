package httpx

import (
	"net/http"
)

var errCrossOrigin = Forbidden("Cross-origin requests are not allowed.")

// CSRF rejects cross-origin state-changing browser requests; non-browser clients pass through
func CSRF(next http.Handler) http.Handler {
	protection := http.NewCrossOriginProtection()
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, errCrossOrigin)
	}))
	return protection.Handler(next)
}
