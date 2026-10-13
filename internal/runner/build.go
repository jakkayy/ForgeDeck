package runner

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/procexec"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

// BuildConfig configures build jobs on this Runner.
type BuildConfig struct {
	// WorkDir holds one temporary workspace per running build.
	WorkDir string
	// GitProtocols is GIT_ALLOW_PROTOCOL for checkouts, e.g. "https".
	GitProtocols string
	// TestLimits bound the test container.
	TestLimits TestLimits
	// Timeout stops a build that runs longer, whatever step it is in.
	Timeout time.Duration
}

// DefaultBuildTimeout is the build time limit unless the operator sets one.
const DefaultBuildTimeout = 20 * time.Minute

const removeImagesTimeout = time.Minute

var (
	errInvalidBuild = &worker.JobError{Code: "invalid_payload", Message: "The build job is not valid for this Runner."}
	errNoCredential = &worker.JobError{Code: "missing_credentials", Message: "The Control Plane sent no repository credentials. Upgrade the Control Plane."}
	errCheckout     = &worker.JobError{Code: "checkout_failed", Message: "The commit could not be fetched. See the build log."}
	errImageBuild   = &worker.JobError{Code: "image_build_failed", Message: "The Docker image could not be built. See the build log."}
	errTests        = &worker.JobError{Code: "tests_failed", Message: "The test command failed. See the build log."}
	errPush         = &worker.JobError{Code: "image_push_failed", Message: "The image could not be pushed to the registry. Check the registry credentials and the build log."}
)

// BuildHandler runs build jobs in a fresh workspace. The workspace and the
// images the build created are removed when the job ends, whatever the
// outcome, so a build host does not fill up.
func BuildHandler(cfg BuildConfig) worker.Handler {
	return func(ctx context.Context, job *agentproto.Job, log worker.Logger) (any, error) {
		var payload agentproto.BuildPayload
		if err := json.Unmarshal(job.Payload, &payload); err != nil || !validCommit.MatchString(payload.CommitSHA) {
			return nil, errInvalidBuild
		}
		if job.Credentials == nil {
			return nil, errNoCredential
		}
		log = worker.NewRedactingLog(log, secretsOf(*job.Credentials))
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
		defer b.removeImages(ctx)

		ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
		result, err := b.run(ctx)
		if err != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, &worker.JobError{Code: "build_timeout",
				Message: "The build took longer than " + cfg.Timeout.String() + " and was stopped."}
		}
		return result, err
	}
}

// build is one build attempt.
type build struct {
	cfg         BuildConfig
	ws          *workspace
	exec        *procexec.Executor
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

	b.log.Log("system", "Building "+b.payload.DockerfilePath+" in "+b.payload.BuildContext)
	if err := buildImage(ctx, b.exec, b.ws.src, b.payload.DockerfilePath, b.payload.BuildContext,
		b.payload.CommitSHA, b.localTag()); err != nil {
		b.log.Log("system", err.Error())
		return nil, errImageBuild
	}

	if b.payload.TestCommand != nil {
		b.log.Log("system", "Running tests in the built image (no network)")
		if err := runTests(ctx, b.exec, b.localTag(), b.testContainer(), *b.payload.TestCommand, b.cfg.TestLimits); err != nil {
			b.log.Log("system", err.Error())
			return nil, errTests
		}
	}

	image, tag := b.credentials.Image, b.payload.CommitSHA[:12]
	b.log.Log("system", "Pushing "+image+":"+tag)
	digest, err := pushImage(ctx, b.exec, b.credentials.Registry, b.localTag(), image, tag)
	if err != nil {
		b.log.Log("system", err.Error())
		return nil, errPush
	}
	b.log.Log("system", "Pushed "+image+"@"+digest)
	return agentproto.BuildResult{CommitSHA: b.payload.CommitSHA, Image: image, Tag: tag, Digest: digest, ImageRef: image + "@" + digest}, nil
}

// removeImages deletes the build's tags from the build host. The pushed
// image stays in the registry; layers shared with other images stay cached.
func (b *build) removeImages(ctx context.Context) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), removeImagesTimeout)
	defer cancel()
	tags := []string{b.localTag()}
	if b.credentials.Image != "" {
		tags = append(tags, b.credentials.Image+":"+b.payload.CommitSHA[:12])
	}
	// Tags that were never created make docker exit non-zero; that is fine.
	_, _ = b.exec.Output(ctx, procexec.Command{Name: "docker", Args: append([]string{"image", "rm", "--force"}, tags...)})
}

// testContainer names the test container of this build.
func (b *build) testContainer() string {
	return "forgedeck-test-" + filepath.Base(b.ws.root)
}

// localTag names the image on the build host until it is pushed. It is
// unique per workspace, so concurrent builds never share a tag.
func (b *build) localTag() string {
	return "forgedeck-build:" + filepath.Base(b.ws.root)
}
