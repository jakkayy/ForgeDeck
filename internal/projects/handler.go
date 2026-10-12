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
	r.Get("/projects/{projectID}", httpx.Handle(h.get))
	r.Patch("/projects/{projectID}", httpx.Handle(h.update))
	r.Delete("/projects/{projectID}", httpx.Handle(h.delete))
	r.Get("/projects/{projectID}/members", httpx.Handle(h.listMembers))
	r.Post("/projects/{projectID}/members", httpx.Handle(h.addMember))
	r.Delete("/projects/{projectID}/members/{userID}", httpx.Handle(h.removeMember))
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

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	project, err := h.svc.Get(r.Context(), auth.UserFrom(r.Context()).ID, projectID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, project)
}

type updateRequest struct {
	Name *string `json:"name"`
	Slug *string `json:"slug"`
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	var req updateRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	project, err := h.svc.Update(r.Context(), auth.UserFrom(r.Context()).ID, projectID, UpdateInput(req))
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, project)
}

type deleteRequest struct {
	Confirm string `json:"confirm"`
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	var req deleteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := h.svc.Delete(r.Context(), auth.UserFrom(r.Context()).ID, projectID, req.Confirm); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) listMembers(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		return err
	}
	members, err := h.svc.ListMembers(r.Context(), auth.UserFrom(r.Context()).ID, projectID, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, httpx.NewList(members, page))
}

type addMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	var req addMemberRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	member, err := h.svc.AddMember(r.Context(), auth.UserFrom(r.Context()).ID, projectID, req.Email, req.Role)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusCreated, member)
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	memberID, err := httpx.UUIDParam(r, "userID")
	if err != nil {
		return err
	}
	if err := h.svc.RemoveMember(r.Context(), auth.UserFrom(r.Context()).ID, projectID, memberID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
