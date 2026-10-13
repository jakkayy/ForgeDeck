package httpx

import "net/http"

// reactAriaPressStyle is the hash of the one inline <style> element that
// react-aria's press handling adds at run time (`touch-action` for pressable
// elements). Allowing exactly that text, instead of 'unsafe-inline', keeps
// every other inline style blocked. The same rule is in the dashboard's own
// CSS, so if a react-aria upgrade changes the text and this hash stops
// matching, behavior is unchanged and the browser console reports a blocked
// style: take the new hash from that message.
const reactAriaPressStyle = "'sha256-38RhXrc7EdReTKsOm23ZPOCUgniTUUcjky8QOOrQx6o='"

// contentSecurityPolicy lets the page load only what ForgeDeck itself serves.
// There is no inline script, no third-party origin, no framing, and no plugin
// content: a cross-site scripting bug in the dashboard cannot load attacker
// code, send data to another origin, or be framed for clickjacking.
const contentSecurityPolicy = "default-src 'none'; " +
	"script-src 'self'; " +
	"style-src 'self' " + reactAriaPressStyle + "; " +
	"img-src 'self' data:; " +
	"font-src 'self'; " +
	"connect-src 'self'; " +
	"manifest-src 'self'; " +
	"base-uri 'none'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'; " +
	"object-src 'none'"

// hstsValue is two years, the value browsers' preload lists expect.
const hstsValue = "max-age=63072000; includeSubDomains"

// SecurityHeaders sets the browser security headers on every response.
// hsts adds Strict-Transport-Security; enable it only when ForgeDeck is
// served over HTTPS, since a browser that sees it refuses plain HTTP to the
// host for the whole period.
func SecurityHeaders(hsts bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Security-Policy", contentSecurityPolicy)
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			h.Set("Cross-Origin-Resource-Policy", "same-origin")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), interest-cohort=()")
			if hsts {
				h.Set("Strict-Transport-Security", hstsValue)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// NoStore marks responses as not cacheable. API responses carry session-
// scoped data, so neither the browser nor a proxy may keep them.
func NoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
