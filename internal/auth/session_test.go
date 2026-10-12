package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/testdb"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

type sessionFixture struct {
	svc   *auth.Service
	clock *fakeClock
	user  store.User
}

func newSessionFixture(t *testing.T) sessionFixture {
	t.Helper()
	clock := &fakeClock{now: time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)}
	svc := auth.NewService(store.New(testdb.New(t)), clock.Now)
	user, err := svc.CreateUser(context.Background(), "dev@example.com", "long-enough-password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return sessionFixture{svc: svc, clock: clock, user: user}
}

func (f sessionFixture) start(t *testing.T) string {
	t.Helper()
	session, err := f.svc.StartSession(context.Background(), f.user.ID)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	return session.Token
}

func (f sessionFixture) authenticate(token string) error {
	_, err := f.svc.Authenticate(context.Background(), token)
	return err
}

func TestSessionAuthenticatesUntilEnded(t *testing.T) {
	f := newSessionFixture(t)
	token := f.start(t)

	user, err := f.svc.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if user.ID != f.user.ID || user.Email != "dev@example.com" {
		t.Fatalf("got %+v", user)
	}

	if err := f.svc.EndSession(context.Background(), token); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	if err := f.authenticate(token); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("after EndSession: got %v, want ErrInvalidSession", err)
	}
}

func TestSessionRejectsUnknownToken(t *testing.T) {
	f := newSessionFixture(t)
	if err := f.authenticate("fds_not-a-real-token"); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("got %v, want ErrInvalidSession", err)
	}
}

func TestSessionExpiresAfterIdleTimeout(t *testing.T) {
	f := newSessionFixture(t)
	token := f.start(t)

	f.clock.Advance(7*24*time.Hour + time.Second)
	if err := f.authenticate(token); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("got %v, want ErrInvalidSession", err)
	}
}

func TestSessionActivityExtendsIdleTimeout(t *testing.T) {
	f := newSessionFixture(t)
	token := f.start(t)

	f.clock.Advance(6 * 24 * time.Hour)
	if err := f.authenticate(token); err != nil {
		t.Fatalf("day 6: %v", err)
	}
	f.clock.Advance(6 * 24 * time.Hour)
	if err := f.authenticate(token); err != nil {
		t.Fatalf("day 12 (sliding expiry): %v", err)
	}
}

func TestSessionEndsAtMaxLifetime(t *testing.T) {
	f := newSessionFixture(t)
	token := f.start(t)

	for day := 6; day < 30; day += 6 {
		f.clock.Advance(6 * 24 * time.Hour)
		if err := f.authenticate(token); err != nil {
			t.Fatalf("day %d: %v", day, err)
		}
	}
	f.clock.Advance(6*24*time.Hour + time.Second) // day 30 + 1s
	if err := f.authenticate(token); !errors.Is(err, auth.ErrInvalidSession) {
		t.Fatalf("after 30 days: got %v, want ErrInvalidSession", err)
	}
}
