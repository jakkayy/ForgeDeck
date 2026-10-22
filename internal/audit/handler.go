package audit

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

// Handler serves the audit log endpoint behind auth.RequireUser
type Handler struct {
	reader *Reader
}

// NewHandler returns the audit log endpoint
func NewHandler(reader *Reader) *Handler {
	return &Handler{reader: reader}
}

// Routes mounts the endpoint on r
func (h *Handler) Routes(r chi.Router) {
	r.Get("/projects/{projectID}/audit-logs", httpx.Handle(h.list))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	projectID, err := httpx.UUIDParam(r, "projectID")
	if err != nil {
		return err
	}
	limit, err := httpx.ParseLimit(r)
	if err != nil {
		return err
	}
	items, next, err := h.reader.List(r.Context(), auth.UserFrom(r.Context()).ID, projectID,
		r.URL.Query().Get("action"), r.URL.Query().Get("cursor"), limit)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, httpx.List[Entry]{Items: items, NextCursor: next})
}
