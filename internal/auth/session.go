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
	sessionIdleTimeout = 7 * 24 * time.Hour
	sessionMaxLifetime = 30 * 24 * time.Hour
	// sessionTouchInterval limits how often activity is written back
	sessionTouchInterval = time.Hour

	sessionTokenPrefix = "fds_"
)

// ErrInvalidSession means the session token is unknown, expired, or signed out
var ErrInvalidSession = errors.New("invalid session")

// User is the authenticated user attached to a request
type User struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
	// IsAdmin may manage what belongs to the whole installation, such as build Runners.
	IsAdmin bool `json:"is_admin"`
}

// NewSession is a started session
type NewSession struct {
	Token     string
	ExpiresAt time.Time
}

// StartSession creates a session for userID
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

// Authenticate resolves a session token to its user and slides the idle expiry, capped at max lifetime
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
	return User{ID: session.UserID, Email: session.Email, IsAdmin: session.IsAdmin}, nil
}

// EndSession deletes the session for token
func (s *Service) EndSession(ctx context.Context, token string) error {
	if err := s.queries.DeleteSessionByTokenHash(ctx, securetoken.Hash(token)); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
