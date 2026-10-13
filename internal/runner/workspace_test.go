package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceIsPrivateAndRemovable(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "work")
	ws, err := newWorkspace(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{ws.root, ws.src, ws.dockerConfig, ws.home} {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
			t.Fatalf("%s: %v %v", dir, info, err)
		}
	}
	if err := os.WriteFile(filepath.Join(ws.dockerConfig, "config.json"), []byte(`{"auths":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := ws.remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ws.root); !os.IsNotExist(err) {
		t.Fatalf("workspace still exists: %v", err)
	}
}

func TestRemoveStaleWorkspaces(t *testing.T) {
	parent := t.TempDir()
	first, _ := newWorkspace(parent)
	second, _ := newWorkspace(parent)
	unrelated := filepath.Join(parent, "cache")
	if err := os.Mkdir(unrelated, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := RemoveStaleWorkspaces(parent); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{first.root, second.root} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("%s survived", dir)
		}
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated directory removed: %v", err)
	}
}
