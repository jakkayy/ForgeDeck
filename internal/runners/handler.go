package runners

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

// Handler serves the Runner management endpoints behind auth.RequireUser.
// Runners belong to the whole installation, so only an administrator may use them.
type Handler struct {
	svc *Service
}

// NewHandler returns the Runner management endpoints.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes mounts the endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/runners", httpx.Handle(h.list))
	r.Post("/runners", httpx.Handle(h.create))
	r.Delete("/runners/{runnerID}", httpx.Handle(h.delete))
}

func requireAdmin(r *http.Request) error {
	if !auth.UserFrom(r.Context()).IsAdmin {
		return httpx.Forbidden("Only an administrator of this ForgeDeck can manage Runners.")
	}
	return nil
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	items, err := h.svc.List(r.Context())
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

type createRequest struct {
	Name string `json:"name"`
}

// createResponse flattens the Runner so the audit log can name it, and adds the
// credential: shown once, never stored in a readable form.
type createResponse struct {
	Runner
	Credential string `json:"credential"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	var req createRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	identity, credential, err := h.svc.Create(r.Context(), req.Name)
	switch {
	case errors.Is(err, ErrInvalidName):
		return validate.Errors{"name": "Use lowercase letters, digits and hyphens, such as build-01."}
	case errors.Is(err, ErrNameTaken):
		return apperr.Conflict("runner_name_taken", "A Runner with this name already exists.")
	case err != nil:
		return err
	}
	runner, err := h.svc.Get(r.Context(), identity.ID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusCreated, createResponse{Runner: runner, Credential: credential})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	id, err := httpx.UUIDParam(r, "runnerID")
	if err != nil {
		return err
	}
	if err := h.svc.DeleteByID(r.Context(), id); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperr.NotFound("Runner not found.")
		}
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
