package worker

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

const defaultHeartbeatInterval = 20 * time.Second

// StatusFunc builds the status part of a heartbeat (Docker and resources).
type StatusFunc func(ctx context.Context) (agentproto.Docker, *agentproto.Resources)

// Heartbeater reports liveness on the interval the Control Plane asks for.
type Heartbeater struct {
	client  *Client
	version string
	status  StatusFunc
	logger  *slog.Logger
	rotate  func(ctx context.Context) error
}

// OnRotate sets what to do when the Control Plane asks for a new credential.
// A failure is logged and retried at the next heartbeat.
func (h *Heartbeater) OnRotate(rotate func(ctx context.Context) error) *Heartbeater {
	h.rotate = rotate
	return h
}

// NewHeartbeater returns a heartbeat loop for client.
func NewHeartbeater(client *Client, version string, status StatusFunc, logger *slog.Logger) *Heartbeater {
	return &Heartbeater{client: client, version: version, status: status, logger: logger}
}

// Run sends heartbeats until ctx ends. It returns ErrUnauthorized when the
// credential is rejected, which means the server or Runner was removed. Other errors
// are logged and retried on the next tick.
func (h *Heartbeater) Run(ctx context.Context) error {
	interval := defaultHeartbeatInterval
	for {
		docker, resources := h.status(ctx)
		resp, err := h.client.Heartbeat(ctx, agentproto.HeartbeatRequest{
			AgentVersion:    h.version,
			ProtocolVersion: agentproto.ProtocolVersion,
			Docker:          docker,
			Resources:       resources,
		})
		switch {
		case errors.Is(err, ErrUnauthorized):
			return err
		case err != nil && ctx.Err() == nil:
			h.logger.Warn("heartbeat failed", "error", err)
		case err == nil && resp.IntervalSeconds > 0:
			interval = time.Duration(resp.IntervalSeconds) * time.Second
		}
		if err == nil && resp.RotateCredential && h.rotate != nil {
			if err := h.rotate(ctx); err != nil && ctx.Err() == nil {
				h.logger.Warn("credential rotation failed; will retry", "error", err)
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(Jitter(interval)):
		}
	}
}

// Jitter spreads workers out by ±10% so they do not report in lockstep.
func Jitter(d time.Duration) time.Duration {
	spread := d / 10
	return d - spread + rand.N(2*spread+1) //nolint:gosec // timing jitter, not security sensitive
}
