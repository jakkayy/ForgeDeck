package deployments

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

// Handler serves the deployment endpoints behind auth.RequireUser.
type Handler struct {
	svc *Service
}

// NewHandler returns deployment endpoints.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes mounts the deployment endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/environments/{environmentID}/deployments", httpx.Handle(h.list))
	r.Post("/environments/{environmentID}/deployments", httpx.Handle(h.deploy))
	r.Get("/deployments/{deploymentID}", httpx.Handle(h.get))
	r.Get("/environments/{environmentID}/releases", httpx.Handle(h.listReleases))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	environmentID, err := httpx.UUIDParam(r, "environmentID")
	if err != nil {
		return err
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		return err
	}
	items, err := h.svc.List(r.Context(), auth.UserFrom(r.Context()).ID, environmentID, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, httpx.NewList(items, page))
}

type deployRequest struct {
	BuildJobID uuid.UUID `json:"build_job_id"`
}

func (h *Handler) deploy(w http.ResponseWriter, r *http.Request) error {
	environmentID, err := httpx.UUIDParam(r, "environmentID")
	if err != nil {
		return err
	}
	var req deployRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	deployment, err := h.svc.Deploy(r.Context(), auth.UserFrom(r.Context()).ID, environmentID, req.BuildJobID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusAccepted, deployment)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	deploymentID, err := httpx.UUIDParam(r, "deploymentID")
	if err != nil {
		return err
	}
	deployment, err := h.svc.Get(r.Context(), auth.UserFrom(r.Context()).ID, deploymentID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, deployment)
}

func (h *Handler) listReleases(w http.ResponseWriter, r *http.Request) error {
	environmentID, err := httpx.UUIDParam(r, "environmentID")
	if err != nil {
		return err
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		return err
	}
	items, err := h.svc.ListReleases(r.Context(), auth.UserFrom(r.Context()).ID, environmentID, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, httpx.NewList(items, page))
}
