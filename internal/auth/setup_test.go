package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/testdb"
)

func emptyAuthAPI(t *testing.T, options ...auth.Option) authAPI {
	t.Helper()
	svc := auth.NewService(store.New(testdb.New(t)), time.Now)
	router := chi.NewRouter()
	auth.NewHandler(svc, false, options...).Routes(router)
	return authAPI{router: router, svc: svc}
}

func setupRequest(email, password string) *http.Request {
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/auth/setup", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func setupRequired(t *testing.T, api authAPI) bool {
	t.Helper()
	rec := api.do(httptest.NewRequest(http.MethodGet, "/auth/setup", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /auth/setup: %d", rec.Code)
	}
	var body struct{ Required bool }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Required
}

func TestFirstAccountCanBeRegisteredOnceAndSignsIn(t *testing.T) {
	api := emptyAuthAPI(t)
	if !setupRequired(t, api) {
		t.Fatal("a new installation does not offer setup")
	}

	rec := api.do(setupRequest("owner@example.com", "long-enough-password"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("setup: %d %s", rec.Code, rec.Body)
	}
	cookie := sessionCookie(t, rec)
	if !cookie.HttpOnly || cookie.Value == "" {
		t.Fatalf("session cookie %+v", cookie)
	}
	me := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	me.AddCookie(cookie)
	if got := api.do(me); got.Code != http.StatusOK {
		t.Fatalf("the new session does not work: %d", got.Code)
	}
	if api.do(loginRequest("owner@example.com", "long-enough-password")).Code != http.StatusOK {
		t.Fatal("the account cannot sign in afterwards")
	}

	if setupRequired(t, api) {
		t.Fatal("setup is still offered after the first account")
	}
	again := api.do(setupRequest("intruder@example.com", "another-long-password"))
	if again.Code != http.StatusConflict {
		t.Fatalf("second setup: %d, want 409", again.Code)
	}
	if api.do(loginRequest("intruder@example.com", "another-long-password")).Code == http.StatusOK {
		t.Fatal("a second account was created")
	}
}

func TestInvalidSetupDoesNotUseUpTheChance(t *testing.T) {
	api := emptyAuthAPI(t)
	for _, bad := range []struct{ email, password string }{
		{"not-an-email", "long-enough-password"},
		{"owner@example.com", "short"},
	} {
		if rec := api.do(setupRequest(bad.email, bad.password)); rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("%+v: %d, want 422", bad, rec.Code)
		}
	}
	if !setupRequired(t, api) {
		t.Fatal("a rejected attempt closed the setup")
	}
	if rec := api.do(setupRequest("owner@example.com", "long-enough-password")); rec.Code != http.StatusCreated {
		t.Fatalf("valid setup after rejected ones: %d", rec.Code)
	}
}

func TestSetupIsClosedOnceAnyAccountExists(t *testing.T) {
	api := emptyAuthAPI(t)
	if _, err := api.svc.CreateUser(context.Background(), "made-by-cli@example.com", "long-enough-password"); err != nil {
		t.Fatal(err)
	}
	if setupRequired(t, api) {
		t.Fatal("setup is offered although an account exists")
	}
	if rec := api.do(setupRequest("late@example.com", "long-enough-password")); rec.Code != http.StatusConflict {
		t.Fatalf("setup after create-user: %d, want 409", rec.Code)
	}
}

func TestOnlyOneOfManySimultaneousSetupsWins(t *testing.T) {
	api := emptyAuthAPI(t)
	const attempts = 12
	codes := make([]int, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			email := string(rune('a'+i)) + "@example.com"
			codes[i] = api.do(setupRequest(email, "long-enough-password")).Code
		}()
	}
	wg.Wait()

	created, refused := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			refused++
		default:
			t.Fatalf("unexpected status %d in %v", code, codes)
		}
	}
	if created != 1 || refused != attempts-1 {
		t.Fatalf("%d created and %d refused, want exactly one winner: %v", created, refused, codes)
	}
}

func TestWebSetupCanBeTurnedOff(t *testing.T) {
	api := emptyAuthAPI(t, auth.WithoutWebSetup())
	if setupRequired(t, api) {
		t.Fatal("setup is offered although it is turned off")
	}
	if rec := api.do(setupRequest("owner@example.com", "long-enough-password")); rec.Code != http.StatusNotFound {
		t.Fatalf("setup while turned off: %d, want 404", rec.Code)
	}
}
