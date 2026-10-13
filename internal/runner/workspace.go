package runner

import (
	"fmt"
	"os"
	"path/filepath"
)

const workspacePattern = "build-*"

// workspace is a build job's private directory tree. Nothing in it outlives
// the job: the checkout, the Docker client config holding registry
// credentials, and the HOME the build commands see.
type workspace struct {
	root         string
	src          string // repository checkout
	dockerConfig string // DOCKER_CONFIG for this job only
	home         string // HOME for build commands, so they never read the Runner's
}

// newWorkspace creates a fresh workspace under parent, readable only by the
// Runner's user.
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

// remove deletes the workspace and everything in it.
func (w *workspace) remove() error {
	if err := os.RemoveAll(w.root); err != nil {
		return fmt.Errorf("remove workspace %s: %w", w.root, err)
	}
	return nil
}

// RemoveStaleWorkspaces deletes workspaces left under parent by a Runner
// that stopped mid-build. Call it before claiming jobs.
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
