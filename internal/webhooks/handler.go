package webhooks

import (
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

// maxPayloadBytes matches GitHub's 25 MB cap with room to spare for GitLab.
const maxPayloadBytes = 25 << 20

// Handler serves provider webhooks. They are authenticated by signature or
// secret token, never by session.
type Handler struct {
	gitHubSecret string // empty when the GitHub App is not configured
}

// NewHandler returns webhook endpoints.
func NewHandler(gitHubSecret string) *Handler {
	return &Handler{gitHubSecret: gitHubSecret}
}

// Routes mounts the webhook endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/webhooks/github", httpx.Handle(h.github))
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

	switch r.Header.Get("X-GitHub-Event") {
	case "ping":
		return httpx.WriteJSON(w, http.StatusOK, map[string]string{"result": "pong"})
	case "push":
		if _, _, _, err := parseGitHubPush(body); err != nil {
			return httpx.BadRequest("invalid_payload", err.Error())
		}
		return httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"result": "accepted"})
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
