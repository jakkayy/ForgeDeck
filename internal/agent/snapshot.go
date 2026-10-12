package agent

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

// snapshotInterval keeps the dashboard's container list well inside the
// Control Plane's two-minute staleness threshold.
const snapshotInterval = 45 * time.Second

// ContainerLister lists the host's containers.
type ContainerLister interface {
	ListContainers(ctx context.Context) ([]agentproto.Container, error)
}

// SnapshotReporter sends the full container list periodically and whenever
// Trigger is called (for example after a container action).
type SnapshotReporter struct {
	client  *Client
	engine  ContainerLister
	logger  *slog.Logger
	trigger chan struct{}
}

// NewSnapshotReporter returns a reporter for engine's containers.
func NewSnapshotReporter(client *Client, engine ContainerLister, logger *slog.Logger) *SnapshotReporter {
	return &SnapshotReporter{client: client, engine: engine, logger: logger, trigger: make(chan struct{}, 1)}
}

// Trigger asks for a snapshot as soon as possible without blocking.
func (r *SnapshotReporter) Trigger() {
	select {
	case r.trigger <- struct{}{}:
	default:
	}
}

// Run reports until ctx ends. It returns ErrUnauthorized when the
// credential is rejected; other failures are logged and retried.
func (r *SnapshotReporter) Run(ctx context.Context) error {
	ticker := time.NewTicker(snapshotInterval)
	defer ticker.Stop()
	for {
		if err := r.report(ctx); errors.Is(err, ErrUnauthorized) {
			return err
		} else if err != nil && ctx.Err() == nil {
			r.logger.Warn("container snapshot failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		case <-r.trigger:
		}
	}
}

func (r *SnapshotReporter) report(ctx context.Context) error {
	observedAt := time.Now()
	containers, err := r.engine.ListContainers(ctx)
	if err != nil {
		return err
	}
	if len(containers) > agentproto.MaxSnapshotContainers {
		r.logger.Warn("too many containers; reporting only the first ones",
			"total", len(containers), "reported", agentproto.MaxSnapshotContainers)
		containers = containers[:agentproto.MaxSnapshotContainers]
	}
	_, err = r.client.Snapshot(ctx, agentproto.ContainerSnapshot{ObservedAt: observedAt, Containers: containers})
	return err
}
