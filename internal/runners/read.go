package runners

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/store"
)

// Status is a Runner's connectivity, derived from its last heartbeat.
type Status string

const (
	// StatusPending means no heartbeat yet.
	StatusPending Status = "pending"
	// StatusOnline means a heartbeat within two intervals.
	StatusOnline Status = "online"
	// StatusOffline means the heartbeats stopped.
	StatusOffline Status = "offline"
)

// heartbeatInterval is how often a Runner reports in (see runnerapi).
const heartbeatInterval = 20 * time.Second

// OnlineAfter is the oldest heartbeat that still counts as online at now.
func OnlineAfter(now time.Time) time.Time { return now.Add(-2 * heartbeatInterval) }

// Runner is the dashboard view of a Runner. It never carries the credential.
type Runner struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Version    *string    `json:"version"`
	Status     Status     `json:"status"`
	LastSeenAt *time.Time `json:"last_seen_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (s *Service) toRunner(row store.Runner) Runner {
	status := StatusPending
	if row.LastSeenAt != nil {
		status = StatusOffline
		if !row.LastSeenAt.Before(OnlineAfter(s.now())) {
			status = StatusOnline
		}
	}
	return Runner{ID: row.ID, Name: row.Name, Version: row.Version, Status: status, LastSeenAt: row.LastSeenAt, CreatedAt: row.CreatedAt}
}

// List returns every Runner by name.
func (s *Service) List(ctx context.Context) ([]Runner, error) {
	rows, err := s.queries.ListRunners(ctx)
	if err != nil {
		return nil, fmt.Errorf("list runners: %w", err)
	}
	out := make([]Runner, len(rows))
	for i, row := range rows {
		out[i] = s.toRunner(row)
	}
	return out, nil
}

// Get returns one Runner.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Runner, error) {
	row, err := s.queries.GetRunner(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Runner{}, ErrNotFound
	}
	if err != nil {
		return Runner{}, fmt.Errorf("get runner: %w", err)
	}
	return s.toRunner(row), nil
}

// Counts returns how many Runners exist and how many are online.
func (s *Service) Counts(ctx context.Context) (total, online int64, err error) {
	row, err := s.queries.CountRunnersByLiveness(ctx, OnlineAfter(s.now()))
	if err != nil {
		return 0, 0, fmt.Errorf("count runners: %w", err)
	}
	return row.Total, row.Online, nil
}

// DeleteByID removes a Runner, revoking its credential immediately.
func (s *Service) DeleteByID(ctx context.Context, id uuid.UUID) error {
	n, err := s.queries.DeleteRunnerByID(ctx, id)
	if err != nil {
		return fmt.Errorf("delete runner: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
