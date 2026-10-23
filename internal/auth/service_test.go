package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/testdb"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

func newService(t *testing.T) *auth.Service {
	t.Helper()
	return auth.NewService(store.New(testdb.New(t)), time.Now)
}

func TestCreateUser(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	user, err := svc.CreateUser(ctx, " dev@example.com ", "long-enough-password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if user.Email != "dev@example.com" {
		t.Errorf("email = %q, want trimmed", user.Email)
	}
	if ok, _ := auth.VerifyPassword("long-enough-password", user.PasswordHash); !ok {
		t.Error("stored hash does not verify")
	}

	_, err = svc.CreateUser(ctx, "DEV@example.com", "another-long-password")
	if !errors.Is(err, auth.ErrEmailTaken) {
		t.Fatalf("duplicate email (different case): got %v, want ErrEmailTaken", err)
	}
}

func TestCreateUserValidatesInput(t *testing.T) {
	svc := newService(t)

	_, err := svc.CreateUser(context.Background(), "Dev <dev@example.com>", "short")
	var fields validate.Errors
	if !errors.As(err, &fields) {
		t.Fatalf("got %v, want validate.Errors", err)
	}
	if fields["email"] == "" || fields["password"] == "" {
		t.Fatalf("missing field errors: %v", fields)
	}
}

func TestFirstAccountIsTheAdministratorAndOthersAreNot(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	first, err := svc.CreateUser(ctx, "first@example.com", "long-enough-password")
	if err != nil || !first.IsAdmin {
		t.Fatalf("first account: admin=%v err=%v", first.IsAdmin, err)
	}
	second, err := svc.CreateUser(ctx, "second@example.com", "long-enough-password")
	if err != nil || second.IsAdmin {
		t.Fatalf("second account: admin=%v err=%v", second.IsAdmin, err)
	}
	third, err := svc.CreateUser(ctx, "third@example.com", "long-enough-password", auth.AsAdmin())
	if err != nil || !third.IsAdmin {
		t.Fatalf("an account made with AsAdmin: admin=%v err=%v", third.IsAdmin, err)
	}

	// The flag follows the session, so /auth/me and every permission check can rely on it.
	session, err := svc.StartSession(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	user, err := svc.Authenticate(ctx, session.Token)
	if err != nil || user.IsAdmin {
		t.Fatalf("authenticated second account: admin=%v err=%v", user.IsAdmin, err)
	}
	session, err = svc.StartSession(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if user, err = svc.Authenticate(ctx, session.Token); err != nil || !user.IsAdmin {
		t.Fatalf("authenticated first account: admin=%v err=%v", user.IsAdmin, err)
	}
}

func TestAccountCreatedFromTheWebIsTheAdministrator(t *testing.T) {
	svc := newService(t)
	user, _, err := svc.RegisterFirstUser(context.Background(), "owner@example.com", "long-enough-password")
	if err != nil || !user.IsAdmin {
		t.Fatalf("first account from the web: admin=%v err=%v", user.IsAdmin, err)
	}
}
