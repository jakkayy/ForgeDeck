package webhooks

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"strconv"
)

// verifyGitLabToken compares X-Gitlab-Token with the connection's secret in
// constant time. GitLab sends the secret itself rather than a signature.
func verifyGitLabToken(secret, header string) error {
	if header == "" || subtle.ConstantTimeCompare([]byte(secret), []byte(header)) != 1 {
		return errBadSignature
	}
	return nil
}

type gitLabPushPayload struct {
	Ref     string `json:"ref"`
	After   string `json:"after"`
	Project struct {
		ID int64 `json:"id"`
	} `json:"project"`
	Commits []struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	} `json:"commits"`
}

// parseGitLabPush returns the push; ok is false for tags and deleted branches.
func parseGitLabPush(body []byte) (push Push, ok bool, err error) {
	var p gitLabPushPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return Push{}, false, errors.New("push payload is not valid JSON")
	}
	branch, isBranch := branchFromRef(p.Ref)
	if !isBranch || isDeletion(p.After) {
		return Push{}, false, nil
	}
	if !validSHA.MatchString(p.After) || p.Project.ID == 0 {
		return Push{}, false, errors.New("push payload is missing the commit or project")
	}
	push = Push{RepositoryID: strconv.FormatInt(p.Project.ID, 10), Branch: branch, CommitSHA: p.After}
	for _, c := range p.Commits {
		if c.ID == p.After {
			push.CommitMessage = firstLine(c.Message)
		}
	}
	return push, true, nil
}
