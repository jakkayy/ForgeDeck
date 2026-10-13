package gitconn

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

// Handler serves the Git connection endpoints.
type Handler struct {
	svc    *Service
	logger *slog.Logger
}

// NewHandler returns Git connection endpoints.
func NewHandler(svc *Service, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, logger: logger}
}

// Routes mounts the endpoints that need a signed-in user.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/projects/{projectID}/git-connections", httpx.Handle(h.list))
	r.Post("/projects/{projectID}/git-connections/github", httpx.Handle(h.startGitHub))
	r.Post("/projects/{projectID}/git-connections/gitlab", httpx.Handle(h.connectGitLab))
	r.Delete("/git-connections/{connectionID}", httpx.Handle(h.delete))
	r.Get("/git-connections/{connectionID}/repositories", httpx.Handle(h.repositories))
}

// PublicRoutes mounts the GitHub setup callback. It runs without a session
// cookie: GitHub's redirect is a cross-site navigation, so the SameSite=Strict
// cookie is not sent. The sealed state identifies the user instead.
func (h *Handler) PublicRoutes(r chi.Router) {
	r.Get("/git/github/setup", h.gitHubSetup)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	connections, err := h.svc.List(r.Context(), auth.UserFrom(r.Context()).ID, projectID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": connections})
}

func (h *Handler) startGitHub(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	installURL, err := h.svc.StartGitHubInstall(r.Context(), auth.UserFrom(r.Context()).ID, projectID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, map[string]string{"install_url": installURL})
}

type gitLabRequest struct {
	BaseURL string `json:"base_url"`
	Token   string `json:"token"`
}

func (h *Handler) connectGitLab(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	var req gitLabRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	conn, err := h.svc.ConnectGitLab(r.Context(), auth.UserFrom(r.Context()).ID, projectID, GitLabInput(req))
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusCreated, conn)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) error {
	connectionID, err := httpx.UUIDParam(r, "connectionID")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(r.Context(), auth.UserFrom(r.Context()).ID, connectionID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) repositories(w http.ResponseWriter, r *http.Request) error {
	connectionID, err := httpx.UUIDParam(r, "connectionID")
	if err != nil {
		return err
	}
	repos, err := h.svc.Repositories(r.Context(), auth.UserFrom(r.Context()).ID, connectionID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": repos})
}

// gitHubSetup finishes an install and sends the browser back to the project
// page, with ?git=connected or ?git_error=<code>.
func (h *Handler) gitHubSetup(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	projectID, err := h.svc.CompleteGitHubInstall(r.Context(), GitHubCallback{
		State:          q.Get("state"),
		InstallationID: q.Get("installation_id"),
		Code:           q.Get("code"),
	})
	if err == nil {
		redirectToProject(w, r, projectID, "git", "connected")
		return
	}

	var installErr *InstallError
	if !errors.As(err, &installErr) {
		installErr = &InstallError{Code: "internal", Err: err}
	}
	h.logger.WarnContext(r.Context(), "github install failed",
		"request_id", httpx.RequestIDFrom(r.Context()), "code", installErr.Code, "error", installErr.Err)
	redirectToProject(w, r, installErr.ProjectID, "git_error", installErr.Code)
}

func redirectToProject(w http.ResponseWriter, r *http.Request, projectID uuid.UUID, key, value string) {
	target := "/projects"
	if projectID != uuid.Nil {
		target += "/" + projectID.String()
	}
	http.Redirect(w, r, target+"?"+url.Values{key: {value}}.Encode(), http.StatusSeeOther)
}
