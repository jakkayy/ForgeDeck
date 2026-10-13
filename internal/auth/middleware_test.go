package auth_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/auth"
)

func protectedAPI(t *testing.T) (authAPI, http.Handler) {
	t.Helper()
	api := newAuthAPI(t, true)
	protected := chi.NewRouter()
	protected.Use(auth.NewHandler(api.svc, true).RequireUser)
	protected.Get("/whoami", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, auth.UserFrom(r.Context()).Email)
	})
	return api, protected
}

func TestRequireUserAllowsValidSession(t *testing.T) {
	api, protected := protectedAPI(t)
	cookie := sessionCookie(t, api.do(loginRequest("dev@example.com", "long-enough-password")))

	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK || rec.Body.String() != "dev@example.com" {
		t.Fatalf("status %d body %q", rec.Code, rec.Body)
	}
}

func TestRequireUserRejectsMissingOrInvalidSession(t *testing.T) {
	_, protected := protectedAPI(t)

	cases := map[string]*http.Cookie{
		"no cookie":     nil,
		"invalid token": {Name: "__Host-forgedeck_session", Value: "fds_forged"},
	}
	for name, cookie := range cases {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
			if cookie != nil {
				req.AddCookie(cookie)
			}
			rec := httptest.NewRecorder()
			protected.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status %d", rec.Code)
			}
		})
	}
}
