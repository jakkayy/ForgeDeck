package app_test

import (
	"net/http"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/config"
)

func TestEveryResponseCarriesSecurityHeaders(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	for _, path := range []string{"/healthz", "/api/v1/auth/me", "/api/v1/projects", "/no/such/page", "/api/v1/no/such/endpoint"} {
		resp := owner.Do(http.MethodGet, path, nil)
		for _, header := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
			if resp.Header.Get(header) == "" {
				t.Errorf("GET %s (status %d) has no %s", path, resp.Status, header)
			}
		}
		if resp.Header.Get("Strict-Transport-Security") != "" {
			t.Errorf("GET %s sent HSTS although cookies are not secure", path)
		}
	}
}

func TestAPIResponsesAreNotCacheable(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	for _, path := range []string{"/api/v1/auth/me", "/api/v1/projects", "/api/v1/no/such/endpoint"} {
		if got := owner.Do(http.MethodGet, path, nil).Header.Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s: Cache-Control = %q, want no-store", path, got)
		}
	}
}

func TestHSTSFollowsTheSecureCookieSetting(t *testing.T) {
	env := apitest.New(t, func(cfg *config.Config) { cfg.CookieSecure = true })
	resp := env.Anonymous().Do(http.MethodGet, "/healthz", nil)
	if resp.Header.Get("Strict-Transport-Security") == "" {
		t.Fatal("no HSTS on an HTTPS deployment")
	}
}
