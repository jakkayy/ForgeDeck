package httpx_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

func serve(handler http.Handler) http.Header {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	return rec.Header()
}

func ok(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func TestSecurityHeadersAreSetOnEveryResponse(t *testing.T) {
	header := serve(httpx.SecurityHeaders(false)(http.HandlerFunc(ok)))
	want := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "no-referrer",
		"Cross-Origin-Opener-Policy":   "same-origin",
		"Cross-Origin-Resource-Policy": "same-origin",
	}
	for name, value := range want {
		if got := header.Get(name); got != value {
			t.Errorf("%s = %q, want %q", name, got, value)
		}
	}
	if header.Get("Permissions-Policy") == "" {
		t.Error("no Permissions-Policy")
	}
	if got := header.Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS sent over plain HTTP: %q", got)
	}
}

func TestContentSecurityPolicyForbidsInlineAndForeignContent(t *testing.T) {
	csp := serve(httpx.SecurityHeaders(false)(http.HandlerFunc(ok))).Get("Content-Security-Policy")
	directives := map[string]string{}
	for _, d := range strings.Split(csp, ";") {
		name, value, _ := strings.Cut(strings.TrimSpace(d), " ")
		directives[name] = value
	}
	// Inline styles are refused except the one react-aria text, named by hash.
	if style := directives["style-src"]; !strings.HasPrefix(style, "'self' 'sha256-") || strings.Count(style, "'sha256-") != 1 {
		t.Errorf("style-src = %q, want 'self' and exactly one hash", style)
	}
	delete(directives, "style-src")
	for name, want := range map[string]string{
		"default-src": "'none'", "script-src": "'self'", "connect-src": "'self'",
		"frame-ancestors": "'none'", "object-src": "'none'", "base-uri": "'none'", "form-action": "'self'",
	} {
		if directives[name] != want {
			t.Errorf("%s = %q, want %q", name, directives[name], want)
		}
	}
	for _, risky := range []string{"unsafe-inline", "unsafe-eval", "*", "http:", "https:"} {
		if strings.Contains(csp, risky) {
			t.Errorf("the policy contains %q: %s", risky, csp)
		}
	}
}

func TestHSTSIsSentOnlyWhenEnabled(t *testing.T) {
	got := serve(httpx.SecurityHeaders(true)(http.HandlerFunc(ok))).Get("Strict-Transport-Security")
	if !strings.Contains(got, "max-age=63072000") || !strings.Contains(got, "includeSubDomains") {
		t.Fatalf("HSTS = %q", got)
	}
}

func TestHeadersSurviveAnErrorResponse(t *testing.T) {
	denied := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "no", http.StatusForbidden) })
	if serve(httpx.SecurityHeaders(false)(denied)).Get("X-Frame-Options") != "DENY" {
		t.Fatal("an error response lost the security headers")
	}
}

func TestNoStore(t *testing.T) {
	if got := serve(httpx.NoStore(http.HandlerFunc(ok))).Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
}
