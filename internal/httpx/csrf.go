package httpx

import (
	"net/http"
)

var errCrossOrigin = Forbidden("Cross-origin requests are not allowed.")

// CSRF rejects state-changing cross-origin browser requests using the
// standard library's Sec-Fetch-Site/Origin checks. Requests without those
// headers (Agents, webhooks, CLI clients) are not browsers and pass through.
// It complements the SameSite=Strict session cookie.
func CSRF(next http.Handler) http.Handler {
	protection := http.NewCrossOriginProtection()
	protection.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, errCrossOrigin)
	}))
	return protection.Handler(next)
}
