// Package runner builds images on a dedicated build host. It claims build
// jobs from the Control Plane over the shared worker protocol.
package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"golang.org/x/sync/errgroup"

	"github.com/jakkayy/ForgeDeck/internal/worker"
)

// Config identifies a Runner to the Control Plane.
type Config struct {
	ControlPlaneURL string
	Credential      string
	Version         string
}

// Run sends heartbeats and runs claimed jobs with handlers until ctx ends or
// the Runner is deleted on the Control Plane.
func Run(ctx context.Context, cfg Config, handlers map[string]worker.Handler, status worker.StatusFunc, logger *slog.Logger) error {
	client := worker.NewClient(cfg.ControlPlaneURL, worker.RunnerAPI, cfg.Credential)
	heartbeater := worker.NewHeartbeater(client, cfg.Version, status, logger)
	jobs := worker.NewWorker(client, handlers, nil, logger)

	logger.Info("runner started", "control_plane", cfg.ControlPlaneURL, "version", cfg.Version)
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error { return heartbeater.Run(ctx) })
	group.Go(func() error { return jobs.Run(ctx) })

	err := group.Wait()
	if errors.Is(err, worker.ErrUnauthorized) {
		return fmt.Errorf("%w: this Runner was deleted from ForgeDeck; create a new one with `forgedeck-api create-runner`", err)
	}
	return err
}
