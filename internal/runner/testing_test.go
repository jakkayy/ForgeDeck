package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// recordLog collects job output for assertions.
type recordLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *recordLog) Log(stream, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, stream+": "+message)
}

func (l *recordLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

// sourceRepo creates a git repository with files committed and returns its
// file:// URL and the commit SHA.
func sourceRepo(t *testing.T, files map[string]string) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // test fixture
			t.Fatal(err)
		}
	}
	git := func(args ...string) string {
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "--quiet")
	git("add", "--all")
	git("commit", "--quiet", "--message", "initial")
	return "file://" + dir, git("rev-parse", "HEAD")
}
