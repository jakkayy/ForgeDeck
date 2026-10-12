package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/securetoken"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

const (
	// sessionIdleTimeout ends a session that has not been used for this long.
	sessionIdleTimeout = 7 * 24 * time.Hour
	// sessionMaxLifetime ends a session this long after sign-in regardless of use.
	sessionMaxLifetime = 30 * 24 * time.Hour
	// sessionTouchInterval limits how often activity is written back, so most
	// authenticated requests are read-only.
	sessionTouchInterval = time.Hour

	sessionTokenPrefix = "fds_"
)

// ErrInvalidSession means the session token is unknown, expired, or signed out.
var ErrInvalidSession = errors.New("invalid session")

// User is the authenticated user attached to a request.
type User struct {
	ID    uuid.UUID
	Email string
}

// NewSession is a started session; Token is shown to the client once.
type NewSession struct {
	Token     string
	ExpiresAt time.Time
}

// StartSession creates a session for userID.
func (s *Service) StartSession(ctx context.Context, userID uuid.UUID) (NewSession, error) {
	now := s.now()
	token, hash := securetoken.New(sessionTokenPrefix)
	session, err := s.queries.CreateSession(ctx, store.CreateSessionParams{
		UserID:    userID,
		TokenHash: hash,
		Now:       now,
		ExpiresAt: now.Add(sessionIdleTimeout),
	})
	if err != nil {
		return NewSession{}, fmt.Errorf("create session: %w", err)
	}
	return NewSession{Token: token, ExpiresAt: session.ExpiresAt}, nil
}

// Authenticate resolves a session token to its user and slides the idle
// expiry forward, capped at the session's maximum lifetime.
func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	now := s.now()
	session, err := s.queries.GetActiveSession(ctx, store.GetActiveSessionParams{
		TokenHash: securetoken.Hash(token),
		Now:       now,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrInvalidSession
	}
	if err != nil {
		return User{}, fmt.Errorf("load session: %w", err)
	}

	if now.Sub(session.LastSeenAt) >= sessionTouchInterval {
		expiresAt := now.Add(sessionIdleTimeout)
		if limit := session.CreatedAt.Add(sessionMaxLifetime); expiresAt.After(limit) {
			expiresAt = limit
		}
		err := s.queries.TouchSession(ctx, store.TouchSessionParams{ID: session.ID, Now: now, ExpiresAt: expiresAt})
		if err != nil {
			return User{}, fmt.Errorf("touch session: %w", err)
		}
	}
	return User{ID: session.UserID, Email: session.Email}, nil
}

// EndSession deletes the session for token. Unknown tokens are ignored.
func (s *Service) EndSession(ctx context.Context, token string) error {
	if err := s.queries.DeleteSessionByTokenHash(ctx, securetoken.Hash(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
