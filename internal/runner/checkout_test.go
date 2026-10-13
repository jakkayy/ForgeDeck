package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/procexec"
)

func TestCheckoutFetchesExactCommit(t *testing.T) {
	url, commit := sourceRepo(t, map[string]string{"Dockerfile": "FROM scratch\n"})
	ws, err := newWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	log := &recordLog{}
	creds := agentproto.GitCredentials{URL: url, Username: "oauth2", Password: "glpat-secret"}

	if err := checkout(context.Background(), newExecutor(ws, log), ws.src, creds, commit, "file"); err != nil {
		t.Fatalf("checkout: %v\n%s", err, log)
	}
	if data, err := os.ReadFile(filepath.Join(ws.src, "Dockerfile")); err != nil || string(data) != "FROM scratch\n" {
		t.Fatalf("Dockerfile %q, %v", data, err)
	}
	config, _ := os.ReadFile(filepath.Join(ws.src, ".git", "config"))
	if strings.Contains(string(config), "glpat-secret") || strings.Contains(log.String(), "glpat-secret") {
		t.Fatal("credential leaked into .git/config or the log")
	}
}

func TestCheckoutRejectsUnknownCommitAndProtocol(t *testing.T) {
	url, _ := sourceRepo(t, map[string]string{"README": "hi\n"})
	cases := map[string]struct{ commit, protocols string }{
		"unknown commit":       {"0123456789abcdef0123456789abcdef01234567", "file"},
		"protocol not allowed": {"0123456789abcdef0123456789abcdef01234567", "https"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ws, _ := newWorkspace(t.TempDir())
			err := checkout(context.Background(), newExecutor(ws, &recordLog{}), ws.src,
				agentproto.GitCredentials{URL: url}, tc.commit, tc.protocols)
			if err == nil {
				t.Fatal("checkout succeeded")
			}
		})
	}
}

func TestExecutorHidesRunnerEnvironment(t *testing.T) {
	t.Setenv("FORGEDECK_RUNNER_CREDENTIAL", "fdr_secret")
	ws, _ := newWorkspace(t.TempDir())
	log := &recordLog{}
	if err := newExecutor(ws, log).Run(context.Background(), procexec.Command{Name: "env"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(log.String(), "fdr_secret") || !strings.Contains(log.String(), "HOME="+ws.home) {
		t.Fatalf("environment:\n%s", log)
	}
}
