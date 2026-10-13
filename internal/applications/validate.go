package applications

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/validate"
)

const (
	maxNameLength        = 100
	maxPathLength        = 255
	maxTestCommandLength = 1000
	maxImageLength       = 255
)

// validImageRepository is the path part of a Docker image reference:
// lowercase components separated by "/", without host or tag.
var validImageRepository = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*(?:/[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*)*$`)

// validBranch is a conservative subset of git-check-ref-format.
var validBranch = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,199}$`)

func checkName(errs validate.Errors, name string) {
	if name == "" || utf8.RuneCountInString(name) > maxNameLength {
		errs.Add("name", fmt.Sprintf("Name must be 1–%d characters.", maxNameLength))
	}
}

func checkBranch(errs validate.Errors, branch string) {
	if !validBranch.MatchString(branch) || strings.Contains(branch, "..") ||
		strings.HasSuffix(branch, "/") || strings.HasSuffix(branch, ".lock") {
		errs.Add("branch", "Use a valid branch name, such as main or release/1.2.")
	}
}

// checkRepoPath accepts a path inside the repository. The Runner resolves it
// against its checkout, so it must not be able to point outside it.
func checkRepoPath(errs validate.Errors, field, value string) {
	cleaned := path.Clean(value)
	if value == "" || len(value) > maxPathLength || path.IsAbs(value) ||
		cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.ContainsAny(value, "\\\x00") {
		errs.Add(field, "Use a path inside the repository, such as Dockerfile or services/api.")
	}
}

func checkTestCommand(errs validate.Errors, command string) {
	if len(command) > maxTestCommandLength || strings.ContainsRune(command, 0) {
		errs.Add("test_command", fmt.Sprintf("Test command must be at most %d characters.", maxTestCommandLength))
	}
}

// checkImage validates a registry and image repository given together and
// returns the repository to store (nil when both are absent).
func checkImage(errs validate.Errors, registryID *uuid.UUID, repository string) *string {
	repository = strings.TrimSpace(repository)
	switch {
	case registryID == nil && repository == "":
		return nil
	case registryID == nil:
		errs.Add("registry_connection_id", "Choose the registry to push this image to.")
	case len(repository) > maxImageLength || !validImageRepository.MatchString(repository):
		errs.Add("image_repository", "Use a lowercase image path without host or tag, such as acme/shop-api.")
	}
	return &repository
}
