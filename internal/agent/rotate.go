package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

// Rotator replaces the Agent's credential when the Control Plane asks
type Rotator struct {
	client *worker.Client
	state  *State
	logger *slog.Logger

	mu       sync.Mutex
	identity Identity
}

// NewRotator returns a Rotator for an enrolled Agent
func NewRotator(client *worker.Client, state *State, identity Identity, logger *slog.Logger) *Rotator {
	return &Rotator{client: client, state: state, identity: identity, logger: logger}
}

// Rotate asks for a new credential, saves it, and only then starts using it
func (r *Rotator) Rotate(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var resp agentproto.RotateResponse
	if err := r.client.Post(ctx, "/rotate-credential", struct{}{}, &resp); err != nil {
		return fmt.Errorf("rotate credential: %w", err)
	}
	next := r.identity
	next.Credential = resp.Credential
	if err := r.state.Save(next); err != nil {
		return fmt.Errorf("save the new credential: %w", err)
	}
	r.identity = next
	r.client.SetCredential(resp.Credential)
	r.logger.Info("agent credential rotated")
	return nil
}
