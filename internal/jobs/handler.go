package jobs

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

// Handler serves the user-facing /jobs endpoints. Routes must be mounted
// behind auth.RequireUser.
type Handler struct {
	svc *Service
}

// NewHandler returns job endpoints.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes mounts the job endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/jobs/{jobID}", httpx.Handle(h.get))
	r.Get("/jobs/{jobID}/logs", httpx.Handle(h.logs))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	jobID, err := httpx.UUIDParam(r, "jobID")
	if err != nil {
		return err
	}
	job, err := h.svc.Get(r.Context(), auth.UserFrom(r.Context()).ID, jobID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, job)
}

// logs pages through stored output with a keyset cursor ("<attempt>.<seq>"),
// so new lines can be fetched as they arrive.
func (h *Handler) logs(w http.ResponseWriter, r *http.Request) error {
	jobID, err := httpx.UUIDParam(r, "jobID")
	if err != nil {
		return err
	}
	page, err := httpx.ParsePage(r)
	if err != nil {
		return err
	}
	after, ok := ParseLogCursor(r.URL.Query().Get("after"))
	if !ok {
		return httpx.BadRequest("invalid_cursor", "after must look like <attempt>.<sequence>.")
	}
	lines, err := h.svc.Logs(r.Context(), auth.UserFrom(r.Context()).ID, jobID, after, page.Limit)
	if err != nil {
		return err
	}

	list := httpx.List[LogLine]{Items: lines}
	if n := len(lines); n > 0 {
		next := LogCursor{Attempt: lines[n-1].Attempt, Sequence: lines[n-1].Sequence}.String()
		list.NextCursor = &next
	}
	return httpx.WriteJSON(w, http.StatusOK, list)
}
