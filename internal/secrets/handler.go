package secrets

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

// Handler serves the secret endpoints behind auth.RequireUser.
type Handler struct {
	svc *Service
}

// NewHandler returns secret endpoints.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes mounts the secret endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/projects/{projectID}/secrets", httpx.Handle(h.list))
	r.Put("/projects/{projectID}/secrets/{name}", httpx.Handle(h.set))
	r.Delete("/projects/{projectID}/secrets/{name}", httpx.Handle(h.delete))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	items, err := h.svc.List(r.Context(), auth.UserFrom(r.Context()).ID, projectID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

type setRequest struct {
	Value string `json:"value"`
}

func (h *Handler) set(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	var req setRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	secret, created, err := h.svc.Set(r.Context(), auth.UserFrom(r.Context()).ID, projectID, chi.URLParam(r, "name"), req.Value)
	if err != nil {
		return err
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	return httpx.WriteJSON(w, status, secret)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(r.Context(), auth.UserFrom(r.Context()).ID, projectID, chi.URLParam(r, "name")); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
