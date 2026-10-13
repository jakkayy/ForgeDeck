package deployments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/audit"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// AutoDeploy is a jobs.SuccessHook: when a build succeeds, it queues a
// deployment into every environment of the application that opted in with
// auto_deploy. It runs in the transaction that records the build's outcome.
//
// Builds of an explicit older commit are not deployed, and neither is a
// build older than what the environment already deploys, because builds can
// finish out of order. A failure in one environment (say, an invalid stored
// configuration) is logged and isolated in a savepoint; it never fails the
// build or the other environments.
func (s *Service) AutoDeploy(logger *slog.Logger) func(context.Context, pgx.Tx, store.Job) error {
	return func(ctx context.Context, tx pgx.Tx, job store.Job) error {
		if job.Type != agentproto.JobBuild || job.ApplicationID == nil {
			return nil
		}
		var payload agentproto.BuildPayload
		if err := unmarshalPayload(job, &payload); err != nil || payload.Trigger == agentproto.TriggerCommit {
			return nil
		}
		result, ok := buildResult(job, *job.ApplicationID)
		if !ok {
			return nil
		}

		q := s.queries.WithTx(tx)
		app, err := q.GetApplication(ctx, *job.ApplicationID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("auto deploy: get application: %w", err)
		}
		envs, err := q.ListAutoDeployEnvironments(ctx, app.ID)
		if err != nil {
			return fmt.Errorf("auto deploy: list environments: %w", err)
		}
		for _, env := range envs {
			if err := s.autoDeployOne(ctx, tx, env, app, job, result); err != nil {
				logger.WarnContext(ctx, "auto deploy skipped", "environment_id", env.ID, "job_id", job.ID, "error", err)
			}
		}
		return nil
	}
}

func (s *Service) autoDeployOne(ctx context.Context, tx pgx.Tx, env store.Environment, app store.Application, job store.Job, result agentproto.BuildResult) (err error) {
	sp, err := tx.Begin(ctx) // a savepoint
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = sp.Rollback(ctx)
			return
		}
		err = sp.Commit(ctx)
	}()

	newer, err := s.queries.WithTx(sp).HasNewerDeployment(ctx, store.HasNewerDeploymentParams{
		EnvironmentID: env.ID, BuildCreatedAt: job.CreatedAt,
	})
	if err != nil || newer {
		return err
	}
	config, err := decodeConfig(env.Config)
	if err != nil {
		return err
	}
	deployment, err := s.startIn(ctx, sp, nil, env, app, source{
		BuildJobID: &job.ID, CommitSHA: result.CommitSHA, ImageRef: result.ImageRef, Config: config,
	})
	if err != nil {
		return err
	}
	// Recorded in the same transaction, so the event exists exactly when the deployment does.
	s.audit.RecordIn(ctx, sp, audit.Event{
		ProjectID: &app.ProjectID, Actor: audit.ActorSystem, Action: "deployment.start", TargetType: "environment",
		TargetID: env.ID.String(), TargetName: env.Name,
		Metadata: map[string]any{"trigger": "auto_deploy", "deployment_id": deployment.ID.String(), "commit": result.CommitSHA},
	})
	return nil
}
