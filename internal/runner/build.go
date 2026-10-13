package runner

import (
	"context"
	"encoding/json"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

// BuildConfig configures build jobs on this Runner.
type BuildConfig struct {
	// WorkDir holds one temporary workspace per running build.
	WorkDir string
	// GitProtocols is GIT_ALLOW_PROTOCOL for checkouts, e.g. "https".
	GitProtocols string
}

// BuildResult is a successful build's job result.
type BuildResult struct {
	CommitSHA string `json:"commit_sha"`
}

var (
	errInvalidBuild = &worker.JobError{Code: "invalid_payload", Message: "The build job is not valid for this Runner."}
	errNoCredential = &worker.JobError{Code: "missing_credentials", Message: "The Control Plane sent no repository credentials. Upgrade the Control Plane."}
	errCheckout     = &worker.JobError{Code: "checkout_failed", Message: "The commit could not be fetched. See the build log."}
)

// BuildHandler runs build jobs in a fresh workspace that is removed when the
// job ends, whatever the outcome.
func BuildHandler(cfg BuildConfig) worker.Handler {
	return func(ctx context.Context, job *agentproto.Job, log worker.Logger) (any, error) {
		var payload agentproto.BuildPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil || !validCommit.MatchString(payload.CommitSHA) {
			return nil, errInvalidBuild
		}
		if job.Credentials == nil {
			return nil, errNoCredential
		}
		ws, err := newWorkspace(cfg.WorkDir)
		if err != nil {
			return nil, err
		}
		defer func() {
			if err := ws.remove(); err != nil {
				log.Log("system", "Could not remove the build workspace: "+err.Error())
			}
		}()

		b := &build{cfg: cfg, ws: ws, exec: newExecutor(ws, log), log: log, payload: payload, credentials: *job.Credentials}
		return b.run(ctx)
	}
}

// build is one build attempt.
type build struct {
	cfg         BuildConfig
	ws          *workspace
	exec        *executor
	log         worker.Logger
	payload     agentproto.BuildPayload
	credentials agentproto.BuildCredentials
}

func (b *build) run(ctx context.Context) (any, error) {
	b.log.Log("system", "Checking out "+b.payload.RepositoryFullName+" at "+b.payload.CommitSHA)
	if err := checkout(ctx, b.exec, b.ws.src, b.credentials.Git, b.payload.CommitSHA, b.cfg.GitProtocols); err != nil {
		b.log.Log("system", err.Error())
		return nil, errCheckout
	}
	return BuildResult{CommitSHA: b.payload.CommitSHA}, nil
}
