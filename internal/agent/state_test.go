package agent

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestStateRoundTripWithPrivatePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	state, err := NewState(dir)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := state.Load(); !errors.Is(err, ErrNotEnrolled) {
		t.Fatalf("empty state: got %v, want ErrNotEnrolled", err)
	}

	want := Identity{
		ControlPlaneURL: "https://forgedeck.example.com",
		AgentID:         uuid.New(),
		ServerID:        uuid.New(),
		Credential:      "fda_secret",
	}
	if err := state.Save(want); err != nil {
		t.Fatal(err)
	}
	got, err := state.Load()
	if err != nil || got != want {
		t.Fatalf("Load = %+v, %v", got, err)
	}

	assertMode(t, dir, 0o700)
	assertMode(t, filepath.Join(dir, identityFile), 0o600)

	leftovers, _ := filepath.Glob(filepath.Join(dir, "*.tmp-*"))
	if len(leftovers) != 0 {
		t.Fatalf("temp files left behind: %v", leftovers)
	}
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s mode %o, want %o", path, got, want)
	}
}
