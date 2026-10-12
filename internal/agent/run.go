package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"github.com/jakkayy/ForgeDeck/internal/agent/docker"
	"github.com/jakkayy/ForgeDeck/internal/agent/host"
	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

// Run starts the heartbeat, snapshot, and job loops for an enrolled Agent
// and blocks until ctx ends or the credential is revoked.
func Run(ctx context.Context, identity Identity, version string, handlers map[string]Handler, engine *docker.Engine, logger *slog.Logger) error {
	client := NewClient(identity.ControlPlaneURL, identity.Credential)
	sampler := host.NewSampler("/")
	status := func(ctx context.Context) (agentproto.Docker, *agentproto.Resources) {
		return engine.Status(ctx), sampler.Sample()
	}

	reporter := NewSnapshotReporter(client, engine, logger)
	heartbeater := NewHeartbeater(client, version, status, logger)
	worker := NewWorker(client, handlers, reporter.Trigger, logger)

	logger.Info("agent started", "server_id", identity.ServerID, "control_plane", identity.ControlPlaneURL, "version", version)
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error { return heartbeater.Run(ctx) })
	group.Go(func() error { return reporter.Run(ctx) })
	group.Go(func() error { return worker.Run(ctx) })

	err := group.Wait()
	if errors.Is(err, ErrUnauthorized) {
		return fmt.Errorf("%w: this server was removed from ForgeDeck; delete the state directory and enroll again", err)
	}
	return err
}
