package app_test

import (
	"net/http"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
)

func TestUnknownAPIPathReturnsJSONNotFound(t *testing.T) {
	env := apitest.New(t)
	resp := env.Anonymous().Do(http.MethodGet, "/api/v1/no-such-endpoint", nil).RequireStatus(http.StatusNotFound)
	if resp.ErrorCode() != "not_found" {
		t.Fatalf("code %q", resp.ErrorCode())
	}
}
