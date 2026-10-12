package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

var errBadSignature = errors.New("webhook signature does not match")

// verifyGitHubSignature checks X-Hub-Signature-256 ("sha256=<hex>") against
// the App's webhook secret in constant time.
func verifyGitHubSignature(secret string, body []byte, header string) error {
	given, ok := strings.CutPrefix(header, "sha256=")
	if !ok {
		return errBadSignature
	}
	signature, err := hex.DecodeString(given)
	if err != nil {
		return errBadSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return errBadSignature
	}
	return nil
}

type gitHubPushPayload struct {
	Ref          string `json:"ref"`
	After        string `json:"after"`
	Deleted      bool   `json:"deleted"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
	Repository struct {
		ID int64 `json:"id"`
	} `json:"repository"`
	HeadCommit *struct {
		Message string `json:"message"`
	} `json:"head_commit"`
}

// parseGitHubPush returns the push and the installation it came through.
// ok is false for pushes that never build: tags, deleted branches.
func parseGitHubPush(body []byte) (push Push, installationID int64, ok bool, err error) {
	var p gitHubPushPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return Push{}, 0, false, errors.New("push payload is not valid JSON")
	}
	branch, isBranch := branchFromRef(p.Ref)
	if !isBranch || p.Deleted || isDeletion(p.After) {
		return Push{}, p.Installation.ID, false, nil
	}
	if !validSHA.MatchString(p.After) || p.Repository.ID == 0 || p.Installation.ID == 0 {
		return Push{}, 0, false, errors.New("push payload is missing the commit, repository, or installation")
	}
	push = Push{RepositoryID: strconv.FormatInt(p.Repository.ID, 10), Branch: branch, CommitSHA: p.After}
	if p.HeadCommit != nil {
		push.CommitMessage = firstLine(p.HeadCommit.Message)
	}
	return push, p.Installation.ID, true, nil
}
