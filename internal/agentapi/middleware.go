package agentapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/servers"
)

type identityKey struct{}

var errAgentUnauthorized = httpx.Unauthorized("Agent credential is invalid or the server was removed. Re-enroll the Agent.")

// RequireAgent authenticates the Authorization: Bearer <credential> header
// and stores the Agent identity in the request context.
func (h *Handler) RequireAgent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		credential := bearerCredential(r)
		if credential == "" {
			httpx.WriteError(w, r, errAgentUnauthorized)
			return
		}
		identity, err := h.servers.AuthenticateAgent(r.Context(), credential)
		if errors.Is(err, servers.ErrInvalidAgentCredential) {
			httpx.WriteError(w, r, errAgentUnauthorized)
			return
		}
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, identity)))
	})
}

// IdentityFrom returns the Agent stored by RequireAgent. It panics on routes
// without RequireAgent, which is a programming error.
func IdentityFrom(ctx context.Context) servers.AgentIdentity {
	identity, ok := ctx.Value(identityKey{}).(servers.AgentIdentity)
	if !ok {
		panic("agentapi.IdentityFrom called on a route without RequireAgent")
	}
	return identity
}

// bearerCredential returns the token from "Authorization: Bearer <token>",
// or "" when the header is missing or uses another scheme.
func bearerCredential(r *http.Request) string {
	credential, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok {
		return ""
	}
	return credential
}
