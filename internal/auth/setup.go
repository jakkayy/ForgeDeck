package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/store"
)

// ErrSetupDone is returned when the first account was already created
var ErrSetupDone = errors.New("setup already completed")

// SetupRequired reports whether no account exists yet, the only time web registration is allowed
func (s *Service) SetupRequired(ctx context.Context) (bool, error) {
	has, err := s.queries.HasUsers(ctx)
	if err != nil {
		return false, fmt.Errorf("check for users: %w", err)
	}
	return !has, nil
}

// RegisterFirstUser creates the first account and signs it in
func (s *Service) RegisterFirstUser(ctx context.Context, email, password string) (User, NewSession, error) {
	email, hash, err := newCredentials(email, password)
	if err != nil {
		return User{}, NewSession{}, err
	}
	record, err := s.queries.CreateFirstUser(ctx, store.CreateFirstUserParams{Email: email, PasswordHash: hash})
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, NewSession{}, ErrSetupDone
	}
	if err != nil {
		return User{}, NewSession{}, fmt.Errorf("create first user: %w", err)
	}
	session, err := s.StartSession(ctx, record.ID)
	if err != nil {
		return User{}, NewSession{}, err
	}
	return User{ID: record.ID, Email: record.Email, IsAdmin: record.IsAdmin}, session, nil
}
