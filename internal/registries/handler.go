package registries

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

// Handler serves the registry endpoints behind auth.RequireUser
type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/projects/{projectID}/registries", httpx.Handle(h.list))
	r.Post("/projects/{projectID}/registries", httpx.Handle(h.create))
	r.Delete("/registries/{registryID}", httpx.Handle(h.delete))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	registries, err := h.svc.List(r.Context(), auth.UserFrom(r.Context()).ID, projectID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": registries})
}

type createRequest struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	var req createRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	registry, err := h.svc.Create(r.Context(), auth.UserFrom(r.Context()).ID, projectID, CreateInput(req))
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusCreated, registry)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) error {
	registryID, err := httpx.UUIDParam(r, "registryID")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(r.Context(), auth.UserFrom(r.Context()).ID, registryID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
