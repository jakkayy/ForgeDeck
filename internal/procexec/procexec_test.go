package procexec_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/procexec"
)

type recorder struct {
	mu    sync.Mutex
	lines []string
}

func (r *recorder) Log(stream, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, stream+": "+message)
}

func (r *recorder) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.lines, "\n")
}

func TestRunLogsOutputByLineAndStream(t *testing.T) {
	log := &recorder{}
	ex := procexec.New(procexec.BaseEnv(t.TempDir(), t.TempDir()), log)
	err := ex.Run(context.Background(), procexec.Command{Name: "sh", Args: []string{"-c", "echo one; echo two >&2; printf partial"}})
	if err != nil {
		t.Fatal(err)
	}
	got := log.String()
	for _, want := range []string{"stdout: one", "stderr: two", "stdout: partial"} {
		if !strings.Contains(got, want) {
			t.Errorf("log missing %q:\n%s", want, got)
		}
	}
}

func TestRunSplitsVeryLongLines(t *testing.T) {
	log := &recorder{}
	ex := procexec.New(procexec.BaseEnv(t.TempDir(), t.TempDir()), log)
	if err := ex.Run(context.Background(), procexec.Command{Name: "sh", Args: []string{"-c", "head -c 40000 /dev/zero | tr '\\0' x"}}); err != nil {
		t.Fatal(err)
	}
	if n := len(log.lines); n < 3 {
		t.Fatalf("a 40000-byte line produced %d log lines", n)
	}
}

func TestCommandsStartWithMinimalEnvironment(t *testing.T) {
	t.Setenv("FORGEDECK_RUNNER_CREDENTIAL", "fdr_secret")
	home := t.TempDir()
	out, err := procexec.New(procexec.BaseEnv(home, t.TempDir()), &recorder{}).
		Output(context.Background(), procexec.Command{Name: "env"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "fdr_secret") || !strings.Contains(out, "HOME="+home) {
		t.Fatalf("environment:\n%s", out)
	}
}

func TestOutputReturnsTrimmedStdoutAndReportsFailure(t *testing.T) {
	ex := procexec.New(procexec.BaseEnv(t.TempDir(), t.TempDir()), &recorder{})
	out, err := ex.Output(context.Background(), procexec.Command{Name: "sh", Args: []string{"-c", "echo '  hi  '"}})
	if err != nil || out != "hi" {
		t.Fatalf("Output = %q, %v", out, err)
	}
	if _, err := ex.Output(context.Background(), procexec.Command{Name: "sh", Args: []string{"-c", "exit 3"}}); err == nil {
		t.Fatal("failure not reported")
	}
}
