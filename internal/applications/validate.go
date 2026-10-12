package applications

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jakkayy/ForgeDeck/internal/validate"
)

const (
	maxNameLength        = 100
	maxPathLength        = 255
	maxTestCommandLength = 1000
)

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
