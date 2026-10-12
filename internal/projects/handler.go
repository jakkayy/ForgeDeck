package projects

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

// Handler serves the /projects endpoints. Routes must be mounted behind
// auth.RequireUser.
type Handler struct {
	svc *Service
}

// NewHandler returns project endpoints.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes mounts the project endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/projects", httpx.Handle(h.list))
	r.Post("/projects", httpx.Handle(h.create))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	page, err := httpx.ParsePage(r)
	if err != nil {
		return err
	}
	projects, err := h.svc.List(r.Context(), auth.UserFrom(r.Context()).ID, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, httpx.NewList(projects, page))
}

type createRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	var req createRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	project, err := h.svc.Create(r.Context(), auth.UserFrom(r.Context()).ID, CreateInput(req))
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusCreated, project)
}
