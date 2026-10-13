// Package procexec runs external programs for Runner and Agent jobs. Commands
// start with a minimal environment, so they never see the credential of the
// process that runs them, and their output goes line by line into the job
// log.
package procexec

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

// passedDockerEnv are the only variables inherited from the parent process
// besides PATH, so the Docker CLI reaches the same daemon as its parent.
var passedDockerEnv = []string{"DOCKER_HOST", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH"}

// BaseEnv is the environment commands start with: PATH, a private HOME and
// DOCKER_CONFIG (so registry logins stay inside one job), and the Docker
// daemon settings of the parent.
func BaseEnv(home, dockerConfig string) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"LANG=C.UTF-8",
		"DOCKER_CONFIG=" + dockerConfig,
	}
	for _, key := range passedDockerEnv {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}

// Command is one process to run.
type Command struct {
	Name  string
	Args  []string
	Dir   string
	Env   []string // added to the executor's base environment
	Stdin io.Reader
}

// Executor runs commands with a fixed base environment, logging their output.
type Executor struct {
	baseEnv []string
	log     worker.Logger
}

// New returns an Executor whose commands start with baseEnv.
func New(baseEnv []string, log worker.Logger) *Executor {
	return &Executor{baseEnv: baseEnv, log: log}
}

// Run executes c, streaming its output to the log, and returns its exit error.
func (e *Executor) Run(ctx context.Context, c Command) error {
	cmd := e.command(ctx, c)
	stdout := &lineWriter{log: e.log, stream: "stdout"}
	stderr := &lineWriter{log: e.log, stream: "stderr"}
	cmd.Stdin = c.Stdin
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	stdout.flush()
	stderr.flush()
	return err
}

// Output executes c and returns its trimmed stdout instead of logging it.
func (e *Executor) Output(ctx context.Context, c Command) (string, error) {
	cmd := e.command(ctx, c)
	stderr := &lineWriter{log: e.log, stream: "stderr"}
	cmd.Stderr = stderr
	out, err := cmd.Output()
	stderr.flush()
	return strings.TrimSpace(string(out)), err
}

func (e *Executor) command(ctx context.Context, c Command) *exec.Cmd {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...) //nolint:gosec // callers choose fixed programs
	cmd.Dir = c.Dir
	cmd.Env = append(slices.Clone(e.baseEnv), c.Env...)
	cmd.WaitDelay = killGrace
	return cmd
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
