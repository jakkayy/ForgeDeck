// Package procexec runs external programs with a minimal environment and streams output to the job log.
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
	// maxLineBytes splits very long lines so one line cannot hold an unbounded buffer
	maxLineBytes = 16 << 10
	// killGrace is how long a killed command may take to exit before its pipes are abandoned
	killGrace = 10 * time.Second
)

// passedDockerEnv are the only variables inherited besides PATH, so the Docker CLI reaches the parent's daemon
var passedDockerEnv = []string{"DOCKER_HOST", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH"}

// BaseEnv is the environment commands start with: PATH, a private HOME and DOCKER_CONFIG, Docker daemon settings
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

type Command struct {
	Name  string
	Args  []string
	Dir   string
	Env   []string // added to the executor's base environment
	Stdin io.Reader
}

type Executor struct {
	baseEnv []string
	log     worker.Logger
}

func New(baseEnv []string, log worker.Logger) *Executor {
	return &Executor{baseEnv: baseEnv, log: log}
}

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
