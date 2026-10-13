package servers

import (
	"net/http"
	"time"

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
	r.Get("/servers", httpx.Handle(h.listForUser))
	r.Get("/servers/{serverID}", httpx.Handle(h.get))
	r.Delete("/servers/{serverID}", httpx.Handle(h.delete))
	r.Get("/servers/{serverID}/containers", httpx.Handle(h.listContainers))
	r.Post("/servers/{serverID}/containers/{containerID}/actions", httpx.Handle(h.containerAction))
	r.Post("/servers/{serverID}/containers/{containerID}/logs", httpx.Handle(h.containerLogs))
	r.Get("/projects/{projectID}/servers", httpx.Handle(h.listForProject))
	r.Post("/projects/{projectID}/servers/enrollment-tokens", httpx.Handle(h.createEnrollmentToken))
}

func (h *Handler) listForUser(w http.ResponseWriter, r *http.Request) error {
	page, err := httpx.ParsePage(r)
	if err != nil {
		return err
	}
	servers, err := h.svc.ListForUser(r.Context(), auth.UserFrom(r.Context()).ID, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, httpx.NewList(servers, page))
}

func (h *Handler) listForProject(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		return err
	}
	servers, err := h.svc.ListForProject(r.Context(), auth.UserFrom(r.Context()).ID, projectID, page.Limit, page.Offset)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, httpx.NewList(servers, page))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	serverID, err := httpx.UUIDParam(r, "serverID")
	if err != nil {
		return err
	}
	server, err := h.svc.Get(r.Context(), auth.UserFrom(r.Context()).ID, serverID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, server)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) error {
	serverID, err := httpx.UUIDParam(r, "serverID")
	if err != nil {
		return err
	}
	if err := h.svc.Delete(r.Context(), auth.UserFrom(r.Context()).ID, serverID); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) listContainers(w http.ResponseWriter, r *http.Request) error {
	serverID, err := httpx.UUIDParam(r, "serverID")
	if err != nil {
		return err
	}
	list, err := h.svc.ListContainers(r.Context(), auth.UserFrom(r.Context()).ID, serverID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, list)
}

type containerActionRequest struct {
	Action string `json:"action"`
}

func (h *Handler) containerAction(w http.ResponseWriter, r *http.Request) error {
	serverID, err := httpx.UUIDParam(r, "serverID")
	if err != nil {
		return err
	}
	var req containerActionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	queued, err := h.svc.RequestContainerAction(r.Context(), auth.UserFrom(r.Context()).ID, serverID,
		chi.URLParam(r, "containerID"), req.Action)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusAccepted, queued)
}

type containerLogsRequest struct {
	TailLines  *int       `json:"tail_lines"`
	Since      *time.Time `json:"since"`
	Timestamps bool       `json:"timestamps"`
}

func (h *Handler) containerLogs(w http.ResponseWriter, r *http.Request) error {
	serverID, err := httpx.UUIDParam(r, "serverID")
	if err != nil {
		return err
	}
	var req containerLogsRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	queued, err := h.svc.RequestContainerLogs(r.Context(), auth.UserFrom(r.Context()).ID, serverID,
		chi.URLParam(r, "containerID"), LogRequest(req))
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusAccepted, queued)
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
