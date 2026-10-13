package runner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

func buildJob(t *testing.T, url, commit string) *agentproto.Job {
	t.Helper()
	payload, err := json.Marshal(agentproto.BuildPayload{RepositoryFullName: "acme/shop", CommitSHA: commit,
		DockerfilePath: "Dockerfile", BuildContext: "."})
	if err != nil {
		t.Fatal(err)
	}
	return &agentproto.Job{Type: agentproto.JobBuild, Payload: payload,
		Credentials: &agentproto.BuildCredentials{Git: agentproto.GitCredentials{URL: url}}}
}

func TestBuildRemovesWorkspaceOnFailure(t *testing.T) {
	url, _ := sourceRepo(t, map[string]string{"Dockerfile": "FROM scratch\n"})
	workDir := t.TempDir()
	handler := BuildHandler(BuildConfig{WorkDir: workDir, GitProtocols: "file", Timeout: time.Minute})

	_, err := handler(context.Background(), buildJob(t, url, "0123456789abcdef0123456789abcdef01234567"), &recordLog{})
	var jobErr *worker.JobError
	if !errors.As(err, &jobErr) || jobErr.Code != "checkout_failed" {
		t.Fatalf("missing commit: %v", err)
	}
	requireEmptyDir(t, workDir)
}

func TestBuildTimeout(t *testing.T) {
	url, commit := sourceRepo(t, map[string]string{"Dockerfile": "FROM scratch\n"})
	workDir := t.TempDir()
	handler := BuildHandler(BuildConfig{WorkDir: workDir, GitProtocols: "file", Timeout: time.Nanosecond})

	_, err := handler(context.Background(), buildJob(t, url, commit), &recordLog{})
	var jobErr *worker.JobError
	if !errors.As(err, &jobErr) || jobErr.Code != "build_timeout" {
		t.Fatalf("got %v, want build_timeout", err)
	}
	requireEmptyDir(t, workDir)
}

func requireEmptyDir(t *testing.T, dir string) {
	t.Helper()
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("workspace left behind: %v", entries)
	}
}

func TestBuildRejectsInvalidJobs(t *testing.T) {
	handler := BuildHandler(BuildConfig{WorkDir: t.TempDir(), GitProtocols: "file", Timeout: time.Minute})
	noCredentials := buildJob(t, "file:///nowhere", "0123456789abcdef0123456789abcdef01234567")
	noCredentials.Credentials = nil
	cases := map[string]struct {
		job  *agentproto.Job
		code string
	}{
		"branch name as commit": {buildJob(t, "file:///nowhere", "main"), "invalid_payload"},
		"no credentials":        {noCredentials, "missing_credentials"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := handler(context.Background(), tc.job, &recordLog{})
			var jobErr *worker.JobError
			if !errors.As(err, &jobErr) || jobErr.Code != tc.code {
				t.Fatalf("got %v, want %s", err, tc.code)
			}
		})
	}
}
