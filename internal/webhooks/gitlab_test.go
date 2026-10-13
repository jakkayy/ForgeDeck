package webhooks

import "testing"

func TestVerifyGitLabToken(t *testing.T) {
	if err := verifyGitLabToken("s3cret", "s3cret"); err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	for _, header := range []string{"", "s3cre", "s3cret ", "other"} {
		if verifyGitLabToken("s3cret", header) == nil {
			t.Errorf("accepted %q", header)
		}
	}
}

func TestParseGitLabPush(t *testing.T) {
	const sha = "da1560886d4f094c3e6c9ef40349f7d38b5d27d7"
	push, ok, err := parseGitLabPush([]byte(`{"ref":"refs/heads/main","after":"` + sha + `",
		"project":{"id":15},"commits":[{"id":"x","message":"older"},{"id":"` + sha + `","message":"Update Catalan\n\nmore"}]}`))
	if err != nil || !ok || push.RepositoryID != "15" || push.Branch != "main" || push.CommitMessage != "Update Catalan" {
		t.Fatalf("push %+v ok=%v err=%v", push, ok, err)
	}
	if _, ok, err := parseGitLabPush([]byte(`{"ref":"refs/heads/x","after":"0000000000000000000000000000000000000000","project":{"id":15}}`)); ok || err != nil {
		t.Errorf("deleted branch: ok=%v err=%v", ok, err)
	}
	if _, _, err := parseGitLabPush([]byte(`{"ref":"refs/heads/x","after":"abc","project":{"id":15}}`)); err == nil {
		t.Error("malformed push accepted")
	}
}
