package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/testdb"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

func newService(t *testing.T) *auth.Service {
	t.Helper()
	return auth.NewService(store.New(testdb.New(t)))
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
