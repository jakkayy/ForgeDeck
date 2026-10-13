package deployments_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/deployments"
)

// deployAndClaim queues a deployment of ref and has the server's Agent claim it.
func (f fixture) deployAndClaim(t *testing.T, ref string) (deployments.Deployment, agentproto.Job) {
	t.Helper()
	result := goodResult()
	result.ImageRef = ref
	buildID := f.build(t, f.appID, "succeeded", result)
	var d deployments.Deployment
	f.deploy(f.owner, f.staging.ID, buildID).RequireStatus(http.StatusAccepted).Decode(&d)
	wait := 0
	var job agentproto.Job
	f.server.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).RequireStatus(http.StatusOK).Decode(&job)
	return d, job
}

func (f fixture) agentComplete(t *testing.T, job agentproto.Job, req agentproto.CompleteRequest) {
	t.Helper()
	req.Attempt = job.Attempt
	f.server.Do(http.MethodPost, "/api/v1/agent/jobs/"+job.ID.String()+"/complete", req).RequireStatus(http.StatusNoContent)
}

func (f fixture) releases(t *testing.T) []deployments.Release {
	t.Helper()
	var list struct{ Items []deployments.Release }
	f.owner.Do(http.MethodGet, "/api/v1/environments/"+f.staging.ID.String()+"/releases", nil).RequireStatus(http.StatusOK).Decode(&list)
	return list.Items
}

func (f fixture) deployment(t *testing.T, id uuid.UUID) deployments.Deployment {
	t.Helper()
	var d deployments.Deployment
	f.owner.Do(http.MethodGet, "/api/v1/deployments/"+id.String(), nil).RequireStatus(http.StatusOK).Decode(&d)
	return d
}

func refOf(hex string) string { return "ghcr.io/acme/shop-api@sha256:" + hex }

var (
	digestA = repeat("a1", 32)
	digestB = repeat("b2", 32)
)

func repeat(s string, n int) string {
	out := ""
	for range n {
		out += s
	}
	return out
}

func TestHealthyDeploymentBecomesTheCurrentRelease(t *testing.T) {
	f := newFixture(t)

	first, job := f.deployAndClaim(t, refOf(digestA))
	if got := f.releases(t); len(got) != 0 {
		t.Fatalf("release before the deployment finished: %+v", got)
	}
	f.agentComplete(t, job, agentproto.CompleteRequest{Outcome: agentproto.OutcomeSucceeded})

	got := f.releases(t)
	if len(got) != 1 || got[0].ImageRef != refOf(digestA) || got[0].DeploymentID != first.ID || !got[0].Current ||
		got[0].CommitSHA != commit || got[0].Config.Env["LOG_LEVEL"] != "info" {
		t.Fatalf("releases %+v", got)
	}
	done := f.deployment(t, first.ID)
	if done.Status != "succeeded" || done.ReleaseID == nil || *done.ReleaseID != got[0].ID || done.Stage != nil {
		t.Fatalf("deployment %+v", done)
	}

	// A newer healthy deployment becomes current; the old release stays.
	_, job = f.deployAndClaim(t, refOf(digestB))
	f.agentComplete(t, job, agentproto.CompleteRequest{Outcome: agentproto.OutcomeSucceeded})
	got = f.releases(t)
	if len(got) != 2 || got[0].ImageRef != refOf(digestB) || !got[0].Current || got[1].ImageRef != refOf(digestA) || got[1].Current {
		t.Fatalf("releases %+v", got)
	}
}

func TestFailedDeploymentIsNeverARelease(t *testing.T) {
	f := newFixture(t)
	d, job := f.deployAndClaim(t, refOf(digestA))
	f.agentComplete(t, job, agentproto.CompleteRequest{
		Outcome: agentproto.OutcomeFailed, ErrorCode: "health_check_failed", ErrorMessage: "did not become healthy",
	})

	if got := f.releases(t); len(got) != 0 {
		t.Fatalf("failed deployment became a release: %+v", got)
	}
	failed := f.deployment(t, d.ID)
	if failed.Status != "failed" || failed.ReleaseID != nil || failed.Stage == nil || *failed.Stage != agentproto.StageHealthCheck {
		t.Fatalf("deployment %+v", failed)
	}
}

func TestFailedStageComesFromTheErrorCode(t *testing.T) {
	cases := map[string]string{
		"pull_failed": agentproto.StagePull, "registry_login_failed": agentproto.StagePull,
		"start_failed": agentproto.StageStart, "health_check_failed": agentproto.StageHealthCheck,
	}
	for code, want := range cases {
		t.Run(code, func(t *testing.T) {
			f := newFixture(t)
			d, job := f.deployAndClaim(t, refOf(digestA))
			f.agentComplete(t, job, agentproto.CompleteRequest{Outcome: agentproto.OutcomeFailed, ErrorCode: code, ErrorMessage: "x"})
			if got := f.deployment(t, d.ID).Stage; got == nil || *got != want {
				t.Fatalf("stage %v, want %s", got, want)
			}
		})
	}
}

func TestRunningDeploymentReportsItsLatestStage(t *testing.T) {
	f := newFixture(t)
	d, job := f.deployAndClaim(t, refOf(digestA))
	if got := f.deployment(t, d.ID); got.Status != "running" || got.Stage != nil {
		t.Fatalf("before any stage line: %+v", got)
	}

	send := func(seq int64, stream, message string) {
		f.server.Do(http.MethodPost, "/api/v1/agent/jobs/"+job.ID.String()+"/events", agentproto.EventsRequest{
			Attempt: job.Attempt, Events: []agentproto.LogEvent{{Sequence: seq, Stream: stream, Message: message}},
		}).RequireStatus(http.StatusNoContent)
	}
	send(1, "system", agentproto.StageLine(agentproto.StagePull, "Pulling the image"))
	send(2, "stdout", "layer abc: Pull complete")
	if got := f.deployment(t, d.ID).Stage; got == nil || *got != agentproto.StagePull {
		t.Fatalf("stage %v", got)
	}
	send(3, "system", agentproto.StageLine(agentproto.StageStart, "Starting the service"))
	send(4, "stdout", "[start] printed by the application, not the Agent")
	if got := f.deployment(t, d.ID).Stage; got == nil || *got != agentproto.StageStart {
		t.Fatalf("stage %v (application output must not count)", got)
	}
}

func TestReleasesAreScopedToProjectMembers(t *testing.T) {
	f := newFixture(t)
	_, job := f.deployAndClaim(t, refOf(digestA))
	f.agentComplete(t, job, agentproto.CompleteRequest{Outcome: agentproto.OutcomeSucceeded})

	outsider, _ := f.env.NewUser("outsider@example.com")
	outsider.Do(http.MethodGet, "/api/v1/environments/"+f.staging.ID.String()+"/releases", nil).RequireStatus(http.StatusNotFound)
	viewer, viewerID := f.env.NewUser("viewer@example.com")
	f.env.AddMember(f.projectID, viewerID, "viewer")
	viewer.Do(http.MethodGet, "/api/v1/environments/"+f.staging.ID.String()+"/releases", nil).RequireStatus(http.StatusOK)
}
