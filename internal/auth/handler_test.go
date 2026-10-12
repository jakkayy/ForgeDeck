package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/testdb"
)

type authAPI struct {
	router http.Handler
	svc    *auth.Service
}

func newAuthAPI(t *testing.T, secureCookies bool) authAPI {
	t.Helper()
	svc := auth.NewService(store.New(testdb.New(t)), time.Now)
	if _, err := svc.CreateUser(context.Background(), "dev@example.com", "long-enough-password"); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	router := chi.NewRouter()
	auth.NewHandler(svc, secureCookies).Routes(router)
	return authAPI{router: router, svc: svc}
}

func (a authAPI) do(req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	a.router.ServeHTTP(rec, req)
	return rec
}

func loginRequest(email, password string) *http.Request {
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if strings.HasSuffix(c.Name, "forgedeck_session") {
			return c
		}
	}
	t.Fatal("no session cookie set")
	return nil
}

func TestLoginSetsSessionCookie(t *testing.T) {
	api := newAuthAPI(t, true)

	rec := api.do(loginRequest("dev@example.com", "long-enough-password"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	cookie := sessionCookie(t, rec)
	if cookie.Name != "__Host-forgedeck_session" || !cookie.HttpOnly || !cookie.Secure ||
		cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Fatalf("insecure cookie attributes: %+v", cookie)
	}
	if _, err := api.svc.Authenticate(context.Background(), cookie.Value); err != nil {
		t.Fatalf("cookie token does not authenticate: %v", err)
	}
	if !strings.Contains(rec.Body.String(), `"email":"dev@example.com"`) {
		t.Fatalf("body = %s", rec.Body)
	}
}

func TestLoginUsesPlainCookieNameWithoutHTTPS(t *testing.T) {
	api := newAuthAPI(t, false)
	cookie := sessionCookie(t, api.do(loginRequest("dev@example.com", "long-enough-password")))
	if cookie.Name != "forgedeck_session" || cookie.Secure {
		t.Fatalf("got %+v", cookie)
	}
}

func TestLoginRejectsBadCredentialsIdentically(t *testing.T) {
	api := newAuthAPI(t, true)

	wrongPassword := api.do(loginRequest("dev@example.com", "not-the-password"))
	unknownEmail := api.do(loginRequest("nobody@example.com", "long-enough-password"))

	for name, rec := range map[string]*httptest.ResponseRecorder{"wrong password": wrongPassword, "unknown email": unknownEmail} {
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status %d", name, rec.Code)
		}
		if len(rec.Result().Cookies()) != 0 {
			t.Errorf("%s: cookie set on failed login", name)
		}
	}
	var a, b map[string]map[string]string
	_ = json.Unmarshal(wrongPassword.Body.Bytes(), &a)
	_ = json.Unmarshal(unknownEmail.Body.Bytes(), &b)
	if a["error"]["message"] != b["error"]["message"] {
		t.Errorf("messages differ: %q vs %q", a["error"]["message"], b["error"]["message"])
	}
}

func TestLogoutEndsSessionAndClearsCookie(t *testing.T) {
	api := newAuthAPI(t, true)
	cookie := sessionCookie(t, api.do(loginRequest("dev@example.com", "long-enough-password")))

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(cookie)
	rec := api.do(req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d", rec.Code)
	}
	if cleared := sessionCookie(t, rec); cleared.MaxAge >= 0 || cleared.Value != "" {
		t.Fatalf("cookie not cleared: %+v", cleared)
	}
	if _, err := api.svc.Authenticate(context.Background(), cookie.Value); err == nil {
		t.Fatal("session still valid after logout")
	}
}

func TestLogoutWithoutSessionSucceeds(t *testing.T) {
	api := newAuthAPI(t, true)
	rec := api.do(httptest.NewRequest(http.MethodPost, "/auth/logout", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d", rec.Code)
	}
}
