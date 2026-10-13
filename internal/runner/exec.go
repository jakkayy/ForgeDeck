package runner

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/worker"
)

const (
	// maxLineBytes splits very long output lines (minified files, progress
	// bars) so one line cannot hold an unbounded buffer.
	maxLineBytes = 16 << 10
	// killGrace is how long a cancelled command may take to exit after
	// being killed before its output pipes are abandoned.
	killGrace = 10 * time.Second
)

// passedDockerEnv are the only Runner environment variables build commands
// inherit, so the Docker CLI reaches the same daemon as the Runner.
var passedDockerEnv = []string{"DOCKER_HOST", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH"}

// command is one process a build step runs.
type command struct {
	name  string
	args  []string
	dir   string
	env   []string // added to the executor's base environment
	stdin io.Reader
}

// executor runs build commands with a minimal environment, streaming their
// output into the job log. Commands never see the Runner's own environment,
// which holds its credential.
type executor struct {
	baseEnv []string
	log     worker.Logger
}

func newExecutor(ws *workspace, log worker.Logger) *executor {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + ws.home,
		"LANG=C.UTF-8",
		"DOCKER_CONFIG=" + ws.dockerConfig,
	}
	for _, key := range passedDockerEnv {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return &executor{baseEnv: env, log: log}
}

// run executes c and returns its exit error, if any.
func (e *executor) run(ctx context.Context, c command) error {
	cmd := exec.CommandContext(ctx, c.name, c.args...) //nolint:gosec // build steps choose fixed programs
	cmd.Dir = c.dir
	cmd.Env = append(slices.Clone(e.baseEnv), c.env...)
	cmd.Stdin = c.stdin
	cmd.WaitDelay = killGrace
	stdout := &lineWriter{log: e.log, stream: "stdout"}
	stderr := &lineWriter{log: e.log, stream: "stderr"}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	stdout.flush()
	stderr.flush()
	return err
}

// output executes c and returns its trimmed stdout instead of logging it.
func (e *executor) output(ctx context.Context, c command) (string, error) {
	cmd := exec.CommandContext(ctx, c.name, c.args...) //nolint:gosec // build steps choose fixed programs
	cmd.Dir = c.dir
	cmd.Env = append(slices.Clone(e.baseEnv), c.env...)
	cmd.Stderr = &lineWriter{log: e.log, stream: "stderr"}
	cmd.WaitDelay = killGrace
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// lineWriter turns a byte stream into log lines.
type lineWriter struct {
	log    worker.Logger
	stream string
	buf    []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.emit(w.buf[:i])
		w.buf = w.buf[i+1:]
	}
	for len(w.buf) > maxLineBytes {
		w.emit(w.buf[:maxLineBytes])
		w.buf = w.buf[maxLineBytes:]
	}
	return len(p), nil
}

func (w *lineWriter) flush() {
	if len(w.buf) > 0 {
		w.emit(w.buf)
		w.buf = nil
	}
}

func (w *lineWriter) emit(line []byte) {
	w.log.Log(w.stream, strings.TrimRight(string(line), "\r"))
}
