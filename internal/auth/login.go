package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
)

// ErrInvalidCredentials is returned for an unknown email or a wrong password, without saying which
var ErrInvalidCredentials = errors.New("invalid email or password")

// dummyHash is verified for unknown emails so timing does not reveal account existence
var dummyHash = sync.OnceValue(func() string {
	hash, err := HashPassword("forgedeck-timing-equalizer")
	if err != nil {
		panic(fmt.Sprintf("hash dummy password: %v", err))
	}
	return hash
})

// Login checks credentials and starts a session
func (s *Service) Login(ctx context.Context, email, password string) (User, NewSession, error) {
	record, err := s.queries.GetUserByEmail(ctx, strings.TrimSpace(email))
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = VerifyPassword(password, dummyHash())
		return User{}, NewSession{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, NewSession{}, fmt.Errorf("load user: %w", err)
	}

	ok, err := VerifyPassword(password, record.PasswordHash)
	if err != nil {
		return User{}, NewSession{}, fmt.Errorf("verify password for user %s: %w", record.ID, err)
	}
	if !ok {
		return User{}, NewSession{}, ErrInvalidCredentials
	}

	session, err := s.StartSession(ctx, record.ID)
	if err != nil {
		return User{}, NewSession{}, err
	}
	return User{ID: record.ID, Email: record.Email}, session, nil
}
