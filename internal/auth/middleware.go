package auth

import (
	"context"
	"errors"
	"net/http"

	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

type userKey struct{}

// RequireUser rejects requests without a valid session and stores the User in the context
func (h *Handler) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := h.cookie.read(r)
		if !ok {
			httpx.WriteError(w, r, errSignInRequired)
			return
		}
		user, err := h.svc.Authenticate(r.Context(), token)
		if errors.Is(err, ErrInvalidSession) {
			h.cookie.clear(w)
			httpx.WriteError(w, r, errSignInRequired)
			return
		}
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, user)))
	})
}

var errSignInRequired = httpx.Unauthorized("Sign in to continue.")

// MaybeUserFrom returns the signed-in user, if RequireUser stored one
func MaybeUserFrom(ctx context.Context) (User, bool) {
	user, ok := ctx.Value(userKey{}).(User)
	return user, ok
}

// UserFrom returns the user stored by RequireUser
func UserFrom(ctx context.Context) User {
	user, ok := ctx.Value(userKey{}).(User)
	if !ok {
		panic("auth.UserFrom called on a route without RequireUser")
	}
	return user
}
