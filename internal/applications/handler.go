package applications

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

// Handler serves the application endpoints behind auth.RequireUser.
type Handler struct {
	svc *Service
}

// NewHandler returns application endpoints.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes mounts the application endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/projects/{projectID}/applications", httpx.Handle(h.list))
	r.Post("/projects/{projectID}/applications", httpx.Handle(h.create))
	r.Get("/applications/{applicationID}", httpx.Handle(h.get))
	r.Patch("/applications/{applicationID}", httpx.Handle(h.update))
	r.Delete("/applications/{applicationID}", httpx.Handle(h.delete))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		return err
	}
	apps, err := h.svc.List(r.Context(), auth.UserFrom(r.Context()).ID, projectID, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, httpx.NewList(apps, page))
}

type createRequest struct {
	Name                 string     `json:"name"`
	GitConnectionID      uuid.UUID  `json:"git_connection_id"`
	RepositoryID         string     `json:"repository_id"`
	Branch               string     `json:"branch"`
	DockerfilePath       string     `json:"dockerfile_path"`
	BuildContext         string     `json:"build_context"`
	TestCommand          string     `json:"test_command"`
	RegistryConnectionID *uuid.UUID `json:"registry_connection_id"`
	ImageRepository      string     `json:"image_repository"`
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
	app, err := h.svc.Create(r.Context(), auth.UserFrom(r.Context()).ID, projectID, CreateInput(req))
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusCreated, app)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	applicationID, err := httpx.UUIDParam(r, "applicationID")
	if err != nil {
		return err
	}
	app, err := h.svc.Get(r.Context(), auth.UserFrom(r.Context()).ID, applicationID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, app)
}

type updateRequest struct {
	Name                 *string    `json:"name"`
	Branch               *string    `json:"branch"`
	DockerfilePath       *string    `json:"dockerfile_path"`
	BuildContext         *string    `json:"build_context"`
	TestCommand          *string    `json:"test_command"`
	RegistryConnectionID *uuid.UUID `json:"registry_connection_id"`
	ImageRepository      *string    `json:"image_repository"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) error {
	applicationID, err := httpx.UUIDParam(r, "applicationID")
	if err != nil {
		return err
	}
	var req updateRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	app, err := h.svc.Update(r.Context(), auth.UserFrom(r.Context()).ID, applicationID, UpdateInput(req))
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, app)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) error {
	applicationID, err := httpx.UUIDParam(r, "applicationID")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(r.Context(), auth.UserFrom(r.Context()).ID, applicationID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
