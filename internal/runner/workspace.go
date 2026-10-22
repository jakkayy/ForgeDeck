package runner

import (
	"fmt"
	"os"
	"path/filepath"
)

const workspacePattern = "build-*"

// workspace is a build job's private directory tree; nothing in it outlives the job
type workspace struct {
	root         string
	src          string // repository checkout
	dockerConfig string // DOCKER_CONFIG for this job only
	home         string // HOME for build commands
}

// newWorkspace creates a fresh workspace under parent, readable only by the Runner's user
func newWorkspace(parent string) (*workspace, error) {
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, fmt.Errorf("create work directory: %w", err)
	}
	root, err := os.MkdirTemp(parent, workspacePattern)
	if err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}
	ws := &workspace{
		root:         root,
		src:          filepath.Join(root, "src"),
		dockerConfig: filepath.Join(root, "docker"),
		home:         filepath.Join(root, "home"),
	}
	for _, dir := range []string{ws.src, ws.dockerConfig, ws.home} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			_ = ws.remove()
			return nil, fmt.Errorf("create workspace: %w", err)
		}
	}
	return ws, nil
}

func (w *workspace) remove() error {
	if err := os.RemoveAll(w.root); err != nil {
		return fmt.Errorf("remove workspace %s: %w", w.root, err)
	}
	return nil
}

// RemoveStaleWorkspaces deletes workspaces left by a Runner that stopped mid-build
func RemoveStaleWorkspaces(parent string) error {
	stale, err := filepath.Glob(filepath.Join(parent, workspacePattern))
	if err != nil {
		return err
	}
	for _, dir := range stale {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove stale workspace: %w", err)
		}
	}
	return nil
}
