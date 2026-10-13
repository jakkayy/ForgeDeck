package webhooks

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/gitconn"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// maxPayloadBytes matches GitHub's 25 MB cap with room to spare for GitLab.
const maxPayloadBytes = 25 << 20

// SecretSource looks up the per-connection GitLab webhook secret.
type SecretSource interface {
	GitLabWebhookSecret(ctx context.Context, connectionID uuid.UUID) (string, error)
}

// Handler serves provider webhooks. They are authenticated by signature or
// secret token, never by session.
type Handler struct {
	gitHubSecret string // empty when the GitHub App is not configured
	gitLab       SecretSource
	queries      *store.Queries
}

// NewHandler returns webhook endpoints.
func NewHandler(gitHubSecret string, gitLab SecretSource, queries *store.Queries) *Handler {
	return &Handler{gitHubSecret: gitHubSecret, gitLab: gitLab, queries: queries}
}

// Routes mounts the webhook endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/webhooks/github", httpx.Handle(h.github))
	r.Post("/webhooks/gitlab/{connectionID}", httpx.Handle(h.gitlab))
}

var errBadToken = httpx.Unauthorized("Webhook token is invalid.")

// gitlab answers 401 for both unknown connections and wrong tokens, so the
// endpoint cannot be used to discover connection IDs.
func (h *Handler) gitlab(w http.ResponseWriter, r *http.Request) error {
	connectionID, err := uuid.Parse(chi.URLParam(r, "connectionID"))
	if err != nil {
		return errBadToken
	}
	secret, err := h.gitLab.GitLabWebhookSecret(r.Context(), connectionID)
	if errors.Is(err, gitconn.ErrNoWebhookSecret) {
		return errBadToken
	}
	if err != nil {
		return err
	}
	if err := verifyGitLabToken(secret, r.Header.Get("X-Gitlab-Token")); err != nil {
		return errBadToken
	}
	body, err := readBody(w, r)
	if err != nil {
		return err
	}

	event := r.Header.Get("X-Gitlab-Event")
	if event != "Push Hook" {
		return httpx.WriteJSON(w, http.StatusOK, map[string]string{"result": "ignored: event not used"})
	}
	push, ok, err := parseGitLabPush(body)
	if err != nil {
		return httpx.BadRequest("invalid_payload", err.Error())
	}
	d := delivery{provider: "gitlab", id: r.Header.Get("X-Gitlab-Event-UUID"), eventType: event, connectionID: &connectionID}
	return h.handleOnce(w, r, d, func(context.Context) (string, error) {
		return pushResult(push, ok), nil
	})
}

// pushResult describes a push that has been accepted but not yet acted on.
func pushResult(push Push, buildable bool) string {
	if !buildable {
		return "ignored: not a branch push"
	}
	return "accepted push to " + push.Branch
}

func (h *Handler) github(w http.ResponseWriter, r *http.Request) error {
	if h.gitHubSecret == "" {
		return httpx.NotFound("GitHub is not configured on this ForgeDeck.")
	}
	body, err := readBody(w, r)
	if err != nil {
		return err
	}
	if err := verifyGitHubSignature(h.gitHubSecret, body, r.Header.Get("X-Hub-Signature-256")); err != nil {
		return httpx.Unauthorized("Webhook signature is invalid.")
	}

	switch event := r.Header.Get("X-GitHub-Event"); event {
	case "ping":
		return httpx.WriteJSON(w, http.StatusOK, map[string]string{"result": "pong"})
	case "push":
		push, _, ok, err := parseGitHubPush(body)
		if err != nil {
			return httpx.BadRequest("invalid_payload", err.Error())
		}
		d := delivery{provider: "github", id: r.Header.Get("X-GitHub-Delivery"), eventType: event}
		return h.handleOnce(w, r, d, func(context.Context) (string, error) {
			return pushResult(push, ok), nil
		})
	default:
		return httpx.WriteJSON(w, http.StatusOK, map[string]string{"result": "ignored: event not used"})
	}
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPayloadBytes))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return nil, &httpx.Error{Status: http.StatusRequestEntityTooLarge, Code: "body_too_large", Message: "Webhook payload is too large."}
	}
	return body, err
}
