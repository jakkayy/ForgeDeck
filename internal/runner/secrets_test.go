package runner

import (
	"strings"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/worker"
)

func TestRedactingLogMasksCredentials(t *testing.T) {
	creds := agentproto.BuildCredentials{
		Git:      agentproto.GitCredentials{URL: "https://gitlab.example.com/acme/shop.git", Username: "oauth2", Password: "glpat-secret-token"},
		Registry: agentproto.RegistryCredentials{Host: "ghcr.io", Username: "bot", Password: "ghp_registry"},
	}
	out := &recordLog{}
	log := worker.NewRedactingLog(out, secretsOf(creds))

	log.Log("stdout", "token glpat-secret-token and glpat-secret-token again")
	log.Log("stderr", "Authorization: Basic "+basicAuth("oauth2", "glpat-secret-token"))
	log.Log("stdout", `{"auths":{"ghcr.io":{"auth":"`+basicAuth("bot", "ghp_registry")+`"}}} ghp_registry`)
	log.Log("stdout", "pushing ghcr.io/acme/shop-api")

	got := out.String()
	for _, secret := range secretsOf(creds) {
		if strings.Contains(got, secret) {
			t.Fatalf("secret %q leaked:\n%s", secret, got)
		}
	}
	if !strings.Contains(got, "token ******** and ******** again") || !strings.Contains(got, "pushing ghcr.io/acme/shop-api") {
		t.Fatalf("unexpected output:\n%s", got)
	}
}

func TestRedactingLogIgnoresShortValues(t *testing.T) {
	out := &recordLog{}
	worker.NewRedactingLog(out, []string{"", "abc"}).Log("stdout", "abc def")
	if out.String() != "stdout: abc def" {
		t.Fatalf("got %q", out.String())
	}
}
