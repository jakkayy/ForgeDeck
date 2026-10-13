package runner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

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

func TestBuildRemovesWorkspaceWhateverTheOutcome(t *testing.T) {
	url, commit := sourceRepo(t, map[string]string{"Dockerfile": "FROM scratch\n"})
	workDir := t.TempDir()
	handler := BuildHandler(BuildConfig{WorkDir: workDir, GitProtocols: "file"})

	requireEmpty := func() {
		t.Helper()
		if entries, _ := os.ReadDir(workDir); len(entries) != 0 {
			t.Fatalf("workspace left behind: %v", entries)
		}
	}

	log := &recordLog{}
	if _, err := handler(context.Background(), buildJob(t, url, commit), log); err != nil {
		t.Fatalf("build: %v\n%s", err, log)
	}
	requireEmpty()

	_, err := handler(context.Background(), buildJob(t, url, "0123456789abcdef0123456789abcdef01234567"), &recordLog{})
	var jobErr *worker.JobError
	if !errors.As(err, &jobErr) || jobErr.Code != "checkout_failed" {
		t.Fatalf("missing commit: %v", err)
	}
	requireEmpty()
}

func TestBuildRejectsInvalidJobs(t *testing.T) {
	handler := BuildHandler(BuildConfig{WorkDir: t.TempDir(), GitProtocols: "file"})
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
