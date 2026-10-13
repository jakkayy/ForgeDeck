package runner

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

var validCommit = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// checkout fetches exactly commit into dir, which must be empty. Only that
// commit is downloaded (no history, tags, or submodules). The credential
// reaches git as an HTTP header through GIT_CONFIG_* variables, never as a
// command argument or in the remote URL, so it does not appear in process
// listings or .git/config.
func checkout(ctx context.Context, exec *executor, dir string, git agentproto.GitCredentials, commit, protocols string) error {
	basic := base64.StdEncoding.EncodeToString([]byte(git.Username + ":" + git.Password))
	env := []string{
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_ALLOW_PROTOCOL=" + protocols,
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=http.extraHeader",
		"GIT_CONFIG_VALUE_0=Authorization: Basic " + basic,
		// The header must not follow a redirect to another host.
		"GIT_CONFIG_KEY_1=http.followRedirects",
		"GIT_CONFIG_VALUE_1=false",
	}
	steps := [][]string{
		{"init", "--quiet", dir},
		{"-C", dir, "fetch", "--depth=1", "--no-tags", "--no-recurse-submodules", "--", git.URL, commit},
		{"-C", dir, "checkout", "--quiet", "--detach", "FETCH_HEAD"},
	}
	for _, args := range steps {
		if err := exec.run(ctx, command{name: "git", args: args, env: env}); err != nil {
			return fmt.Errorf("git %s: %w", args[len(args)-1], err)
		}
	}
	head, err := exec.output(ctx, command{name: "git", args: []string{"-C", dir, "rev-parse", "HEAD"}, env: env})
	if err != nil {
		return fmt.Errorf("read checked out commit: %w", err)
	}
	if head != commit {
		return fmt.Errorf("checked out %s, want %s", head, commit)
	}
	return nil
}
