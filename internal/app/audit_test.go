package app_test

import (
	"log/slog"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/app"
	"github.com/jakkayy/ForgeDeck/internal/audit"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/testdb"
)

// A new data-changing route must be audited or exempt with a reason, or this test fails.
func TestEveryMutatingRouteIsAuditedOrExemptWithAReason(t *testing.T) {
	handler := app.NewRouter(t.Context(), config.Config{}, testdb.New(t), slog.New(slog.DiscardHandler))
	routes, ok := handler.(chi.Routes)
	if !ok {
		t.Fatal("the router is not a chi.Routes")
	}

	seen := map[string]bool{}
	err := chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			return nil
		}
		key := method + " " + route
		seen[key] = true
		_, audited := audit.Rules[key]
		reason, exempt := audit.Exempt[key]
		switch {
		case audited && exempt:
			t.Errorf("%s is both audited and exempt", key)
		case !audited && !exempt:
			t.Errorf("%s changes data but is neither in audit.Rules nor in audit.Exempt", key)
		case exempt && reason == "":
			t.Errorf("%s is exempt without a reason", key)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for key := range audit.Rules {
		if !seen[key] {
			t.Errorf("audit.Rules has %s, which is not a route", key)
		}
	}
	for key := range audit.Exempt {
		if !seen[key] {
			t.Errorf("audit.Exempt has %s, which is not a route", key)
		}
	}
	if len(seen) == 0 {
		t.Fatal("no routes found")
	}
}
