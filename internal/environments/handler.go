package environments

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/servicecfg"
)

// Handler serves the environment endpoints behind auth.RequireUser
type Handler struct {
	svc *Service
}

// NewHandler returns environment endpoints
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes mounts the environment endpoints on r
func (h *Handler) Routes(r chi.Router) {
	r.Get("/applications/{applicationID}/environments", httpx.Handle(h.list))
	r.Post("/applications/{applicationID}/environments", httpx.Handle(h.create))
	r.Get("/environments/{environmentID}", httpx.Handle(h.get))
	r.Patch("/environments/{environmentID}", httpx.Handle(h.update))
	r.Delete("/environments/{environmentID}", httpx.Handle(h.delete))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	applicationID, err := httpx.UUIDParam(r, "applicationID")
	if err != nil {
		return err
	}
	items, err := h.svc.List(r.Context(), auth.UserFrom(r.Context()).ID, applicationID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

type createRequest struct {
	Name       string            `json:"name"`
	ServerID   uuid.UUID         `json:"server_id"`
	AutoDeploy bool              `json:"auto_deploy"`
	Config     servicecfg.Config `json:"config"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	applicationID, err := httpx.UUIDParam(r, "applicationID")
	if err != nil {
		return err
	}
	var req createRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	env, err := h.svc.Create(r.Context(), auth.UserFrom(r.Context()).ID, applicationID, CreateInput(req))
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusCreated, env)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	environmentID, err := httpx.UUIDParam(r, "environmentID")
	if err != nil {
		return err
	}
	env, err := h.svc.Get(r.Context(), auth.UserFrom(r.Context()).ID, environmentID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, env)
}

type updateRequest struct {
	Name       *string            `json:"name"`
	ServerID   *uuid.UUID         `json:"server_id"`
	AutoDeploy *bool              `json:"auto_deploy"`
	Config     *servicecfg.Config `json:"config"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) error {
	environmentID, err := httpx.UUIDParam(r, "environmentID")
	if err != nil {
		return err
	}
	var req updateRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	env, err := h.svc.Update(r.Context(), auth.UserFrom(r.Context()).ID, environmentID, UpdateInput(req))
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, env)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) error {
	environmentID, err := httpx.UUIDParam(r, "environmentID")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(r.Context(), auth.UserFrom(r.Context()).ID, environmentID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
