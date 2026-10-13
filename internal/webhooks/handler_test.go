package webhooks_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/config"
)

const (
	webhookSecret = "github-webhook-secret"
	sha           = "0123456789abcdef0123456789abcdef01234567"
)

func gitHubEnv(t *testing.T) *apitest.Env {
	t.Helper()
	return apitest.New(t, func(cfg *config.Config) { cfg.GitHubWebhookSecret = webhookSecret })
}

func signed(body []byte, event, delivery string) map[string]string {
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write(body)
	return map[string]string{
		"X-Hub-Signature-256": "sha256=" + hex.EncodeToString(mac.Sum(nil)),
		"X-GitHub-Event":      event,
		"X-GitHub-Delivery":   delivery,
	}
}

var push = []byte(`{"ref":"refs/heads/main","after":"` + sha + `","installation":{"id":11},"repository":{"id":5}}`)

func TestGitHubWebhookRejectsBadSignature(t *testing.T) {
	env := gitHubEnv(t)
	headers := signed(push, "push", "d-1")
	headers["X-Hub-Signature-256"] = "sha256=" + hex.EncodeToString(make([]byte, 32))
	env.Anonymous().DoRaw(http.MethodPost, "/api/v1/webhooks/github", push, headers).RequireStatus(http.StatusUnauthorized)
}

func TestGitHubWebhookPing(t *testing.T) {
	env := gitHubEnv(t)
	body := []byte(`{"zen":"Keep it logically awesome."}`)
	env.Anonymous().DoRaw(http.MethodPost, "/api/v1/webhooks/github", body, signed(body, "ping", "p-1")).RequireStatus(http.StatusOK)
}

func TestGitHubWebhookDeduplicatesDeliveries(t *testing.T) {
	env := gitHubEnv(t)
	client := env.Anonymous()
	client.DoRaw(http.MethodPost, "/api/v1/webhooks/github", push, signed(push, "push", "d-1")).RequireStatus(http.StatusAccepted)
	client.DoRaw(http.MethodPost, "/api/v1/webhooks/github", push, signed(push, "push", "d-1")).RequireStatus(http.StatusOK)
	client.DoRaw(http.MethodPost, "/api/v1/webhooks/github", push, signed(push, "push", "d-2")).RequireStatus(http.StatusAccepted)

	var events int
	if err := env.Pool.QueryRow(t.Context(), "SELECT count(*) FROM webhook_events WHERE provider = 'github'").Scan(&events); err != nil || events != 2 {
		t.Fatalf("%d events recorded (err %v), want 2", events, err)
	}
}

func TestGitHubWebhookDisabledWithoutApp(t *testing.T) {
	env := apitest.New(t)
	env.Anonymous().DoRaw(http.MethodPost, "/api/v1/webhooks/github", push, signed(push, "push", "d-1")).RequireStatus(http.StatusNotFound)
}

func TestGitLabWebhookRejectsUnknownConnectionAndBadToken(t *testing.T) {
	env := apitest.New(t)
	headers := map[string]string{"X-Gitlab-Token": "guess", "X-Gitlab-Event": "Push Hook", "X-Gitlab-Event-UUID": "u-1"}
	for _, path := range []string{"/api/v1/webhooks/gitlab/" + uuid.NewString(), "/api/v1/webhooks/gitlab/not-a-uuid"} {
		env.Anonymous().DoRaw(http.MethodPost, path, push, headers).RequireStatus(http.StatusUnauthorized)
	}
}
