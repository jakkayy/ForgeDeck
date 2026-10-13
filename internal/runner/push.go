package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/procexec"
)

var validDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// pushImage logs in to the registry, pushes localTag as image:tag, and
// returns the pushed manifest's digest. The password goes to docker on
// stdin and is stored only in the job's own DOCKER_CONFIG.
func pushImage(ctx context.Context, exec *procexec.Executor, registry agentproto.RegistryCredentials, localTag, image, tag string) (string, error) {
	login := procexec.Command{
		Name:  "docker",
		Args:  []string{"login", "--username", registry.Username, "--password-stdin", registry.Host},
		Stdin: strings.NewReader(registry.Password),
	}
	if err := exec.Run(ctx, login); err != nil {
		return "", fmt.Errorf("docker login %s: %w", registry.Host, err)
	}
	remote := image + ":" + tag
	if err := exec.Run(ctx, procexec.Command{Name: "docker", Args: []string{"tag", localTag, remote}}); err != nil {
		return "", fmt.Errorf("docker tag: %w", err)
	}
	if err := exec.Run(ctx, procexec.Command{Name: "docker", Args: []string{"push", remote}}); err != nil {
		return "", fmt.Errorf("docker push: %w", err)
	}
	out, err := exec.Output(ctx, procexec.Command{Name: "docker", Args: []string{"image", "inspect", "--format", "{{json .RepoDigests}}", remote}})
	if err != nil {
		return "", fmt.Errorf("read pushed digest: %w", err)
	}
	return digestFor(out, image)
}

// digestFor picks image's digest from docker's RepoDigests JSON. Docker
// shortens Docker Hub names ("docker.io/library/x" becomes "x"), so names
// are compared in that short form.
func digestFor(repoDigestsJSON, image string) (string, error) {
	var repoDigests []string
	if err := json.Unmarshal([]byte(repoDigestsJSON), &repoDigests); err != nil {
		return "", fmt.Errorf("parse repo digests: %w", err)
	}
	want := shortName(image)
	for _, ref := range repoDigests {
		name, digest, ok := strings.Cut(ref, "@")
		if ok && shortName(name) == want && validDigest.MatchString(digest) {
			return digest, nil
		}
	}
	return "", fmt.Errorf("no digest for %s in %v", image, repoDigests)
}

func shortName(image string) string {
	for _, prefix := range []string{"docker.io/", "index.docker.io/", "registry-1.docker.io/"} {
		image = strings.TrimPrefix(image, prefix)
	}
	return strings.TrimPrefix(image, "library/")
}
