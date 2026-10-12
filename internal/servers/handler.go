package servers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

// Handler serves the user-facing server endpoints. Routes must be mounted
// behind auth.RequireUser.
type Handler struct {
	svc *Service
}

// NewHandler returns server endpoints.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes mounts the server endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/projects/{projectID}/servers/enrollment-tokens", httpx.Handle(h.createEnrollmentToken))
}

type enrollmentTokenRequest struct {
	ServerName string `json:"server_name"`
}

func (h *Handler) createEnrollmentToken(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	var req enrollmentTokenRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	token, err := h.svc.CreateEnrollmentToken(r.Context(), auth.UserFrom(r.Context()).ID, projectID, req.ServerName)
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	return httpx.WriteJSON(w, http.StatusCreated, token)
}
