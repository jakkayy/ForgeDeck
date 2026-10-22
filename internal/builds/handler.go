package builds

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

// Handler serves the build endpoints behind auth.RequireUser
type Handler struct {
	svc *Service
}

// NewHandler returns build endpoints
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes mounts the build endpoints on r
func (h *Handler) Routes(r chi.Router) {
	r.Post("/applications/{applicationID}/builds", httpx.Handle(h.trigger))
}

type triggerRequest struct {
	CommitSHA string `json:"commit_sha"`
}

func (h *Handler) trigger(w http.ResponseWriter, r *http.Request) error {
	applicationID, err := httpx.UUIDParam(r, "applicationID")
	if err != nil {
		return err
	}
	var req triggerRequest
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			return err
		}
	}
	queued, err := h.svc.Trigger(r.Context(), auth.UserFrom(r.Context()).ID, applicationID, req.CommitSHA)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusAccepted, queued)
}
