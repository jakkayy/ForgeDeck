package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyGitHubSignature(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	if err := verifyGitHubSignature("s3cret", body, sign("s3cret", body)); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	for name, header := range map[string]string{
		"wrong secret":  sign("other", body),
		"tampered body": sign("s3cret", []byte(`{"ref":"refs/heads/evil"}`)),
		"sha1 scheme":   "sha1=abc",
		"not hex":       "sha256=zz",
		"missing":       "",
	} {
		if err := verifyGitHubSignature("s3cret", body, header); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestParseGitHubPush(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"
	push, installation, ok, err := parseGitHubPush([]byte(`{
		"ref": "refs/heads/release/1.2", "after": "` + sha + `",
		"installation": {"id": 11}, "repository": {"id": 1296269},
		"head_commit": {"message": "Fix login\n\nLonger body"}}`))
	if err != nil || !ok || installation != 11 {
		t.Fatalf("ok=%v installation=%d err=%v", ok, installation, err)
	}
	if push.Branch != "release/1.2" || push.CommitSHA != sha || push.RepositoryID != "1296269" || push.CommitMessage != "Fix login" {
		t.Fatalf("push %+v", push)
	}

	for name, body := range map[string]string{
		"tag":            `{"ref":"refs/tags/v1","after":"` + sha + `","installation":{"id":11},"repository":{"id":1}}`,
		"branch deleted": `{"ref":"refs/heads/x","after":"0000000000000000000000000000000000000000","deleted":true,"installation":{"id":11},"repository":{"id":1}}`,
	} {
		if _, _, ok, err := parseGitHubPush([]byte(body)); ok || err != nil {
			t.Errorf("%s: ok=%v err=%v, want ignored", name, ok, err)
		}
	}
	if _, _, _, err := parseGitHubPush([]byte(`{"ref":"refs/heads/main","after":"not-a-sha"}`)); err == nil {
		t.Error("malformed push accepted")
	}
}
