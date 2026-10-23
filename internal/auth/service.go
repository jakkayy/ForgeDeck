package auth

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

// ErrEmailTaken is returned when creating a user whose email already exists
var ErrEmailTaken = errors.New("email already registered")

// Service implements user and session operations
type Service struct {
	queries *store.Queries
	now     func() time.Time
}

// NewService returns a Service backed by queries
func NewService(queries *store.Queries, now func() time.Time) *Service {
	return &Service{queries: queries, now: now}
}

// newCredentials validates an email and password and hashes the password
func newCredentials(email, password string) (cleanEmail, hash string, err error) {
	email = strings.TrimSpace(email)
	errs := validate.Errors{}
	if !isEmail(email) {
		errs.Add("email", "Enter a valid email address.")
	}
	if len(password) < MinPasswordLength {
		errs.Add("password", fmt.Sprintf("Password must be at least %d characters.", MinPasswordLength))
	}
	if err := errs.Err(); err != nil {
		return "", "", err
	}
	hash, err = HashPassword(password)
	if err != nil {
		return "", "", err
	}
	return email, hash, nil
}

// UserOption changes how CreateUser makes the account.
type UserOption func(*userOptions)

type userOptions struct{ admin bool }

// AsAdmin makes the new account an administrator; the first account of an installation is one anyway.
func AsAdmin() UserOption { return func(o *userOptions) { o.admin = true } }

// CreateUser validates the input, hashes the password, and stores the user
func (s *Service) CreateUser(ctx context.Context, email, password string, options ...UserOption) (store.User, error) {
	var opts userOptions
	for _, option := range options {
		option(&opts)
	}
	email, hash, err := newCredentials(email, password)
	if err != nil {
		return store.User{}, err
	}
	user, err := s.queries.CreateUser(ctx, store.CreateUserParams{Email: email, PasswordHash: hash, IsAdmin: opts.admin})
	if database.IsUniqueViolation(err) {
		return store.User{}, ErrEmailTaken
	}
	if err != nil {
		return store.User{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

// isEmail accepts a bare address and rejects display names like "Dev <dev@example.com>"
func isEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	return err == nil && addr.Address == s
}
