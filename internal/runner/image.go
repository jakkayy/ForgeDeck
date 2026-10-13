package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jakkayy/ForgeDeck/internal/procexec"
)

// buildImage builds the application's Dockerfile with BuildKit and tags the
// result as tag. BuildKit runs RUN steps in containers without host mounts.
func buildImage(ctx context.Context, exec *procexec.Executor, src, dockerfile, buildContext, commit, tag string) error {
	dockerfilePath, err := resolveInside(src, dockerfile)
	if err != nil {
		return fmt.Errorf("dockerfile: %w", err)
	}
	contextPath, err := resolveInside(src, buildContext)
	if err != nil {
		return fmt.Errorf("build context: %w", err)
	}
	return exec.Run(ctx, procexec.Command{
		Name: "docker",
		Args: []string{
			"build", "--progress=plain",
			"--file", dockerfilePath,
			"--tag", tag,
			"--label", "org.opencontainers.image.revision=" + commit,
			"--", contextPath,
		},
		Env: []string{"DOCKER_BUILDKIT=1"},
	})
}

var errOutsideCheckout = errors.New("path leaves the repository checkout")

// resolveInside resolves a repository-relative path, following symlinks,
// and refuses results outside root. The Control Plane validates paths too,
// but a symlink committed to the repository is only visible here.
func resolveInside(root, rel string) (string, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(rel)))
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%s does not exist in the repository", rel)
	}
	if err != nil {
		return "", err
	}
	if resolved != realRoot && !strings.HasPrefix(resolved, realRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: %w", rel, errOutsideCheckout)
	}
	return resolved, nil
}
