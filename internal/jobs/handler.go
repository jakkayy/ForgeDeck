package jobs

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

// Handler serves the /jobs endpoints; mount behind auth.RequireUser
type Handler struct {
	svc      *Service
	notifier *Notifier
	shutdown context.Context
}

func NewHandler(shutdown context.Context, svc *Service, notifier *Notifier) *Handler {
	return &Handler{svc: svc, notifier: notifier, shutdown: shutdown}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/jobs", httpx.Handle(h.history))
	r.Get("/jobs/{jobID}", httpx.Handle(h.get))
	r.Get("/jobs/{jobID}/logs", httpx.Handle(h.logs))
	r.Post("/jobs/{jobID}/cancel", httpx.Handle(h.cancel))
	r.Get("/jobs/{jobID}/events", h.events)
}

// events streams logs and status as SSE; resumes from Last-Event-ID or ?after=
func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	jobID, err := httpx.UUIDParam(r, "jobID")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	resume := r.Header.Get("Last-Event-ID")
	if resume == "" {
		resume = r.URL.Query().Get("after")
	}
	after, ok := ParseLogCursor(resume)
	if !ok {
		httpx.WriteError(w, r, httpx.BadRequest("invalid_cursor", "Resume position must look like <attempt>.<sequence>."))
		return
	}
	userID := auth.UserFrom(r.Context()).ID
	if err := h.svc.authorizeJobOutput(r.Context(), userID, jobID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer context.AfterFunc(h.shutdown, cancel)()
	_ = h.svc.Stream(ctx, userID, jobID, after, h.notifier, httpx.NewSSE(w))
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) error {
	jobID, err := httpx.UUIDParam(r, "jobID")
	if err != nil {
		return err
	}
	job, err := h.svc.Cancel(r.Context(), auth.UserFrom(r.Context()).ID, jobID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, job)
}

var validStatuses = map[Status]bool{
	StatusQueued: true, StatusRunning: true, StatusSucceeded: true, StatusFailed: true, StatusCancelled: true,
}

// history lists jobs newest first with filters and ?cursor=
func (h *Handler) history(w http.ResponseWriter, r *http.Request) error {
	limit, err := httpx.ParseLimit(r)
	if err != nil {
		return err
	}
	q := r.URL.Query()
	after, ok := DecodeHistoryCursor(q.Get("cursor"))
	if !ok {
		return httpx.BadRequest("invalid_cursor", "cursor is not valid.")
	}
	var filter Filter
	for name, dst := range map[string]**uuid.UUID{
		"project_id": &filter.ProjectID, "application_id": &filter.ApplicationID, "server_id": &filter.ServerID,
	} {
		if raw := q.Get(name); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				return httpx.BadRequest("invalid_filter", name+" must be a UUID.")
			}
			*dst = &id
		}
	}
	if raw := q.Get("status"); raw != "" {
		status := Status(raw)
		if !validStatuses[status] {
			return httpx.BadRequest("invalid_filter", "status must be queued, running, succeeded, failed or cancelled.")
		}
		filter.Status = &status
	}
	if raw := q.Get("type"); raw != "" {
		filter.Type = &raw
	}

	jobs, next, err := h.svc.History(r.Context(), auth.UserFrom(r.Context()).ID, filter, after, limit)
	if err != nil {
		return err
	}
	list := httpx.List[Job]{Items: jobs}
	if next != nil {
		cursor := next.Encode()
		list.NextCursor = &cursor
	}
	return httpx.WriteJSON(w, http.StatusOK, list)
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

// logs pages through stored output with a keyset cursor "<attempt>.<seq>"
func (h *Handler) logs(w http.ResponseWriter, r *http.Request) error {
	jobID, err := httpx.UUIDParam(r, "jobID")
	if err != nil {
		return err
	}
	limit, err := httpx.ParseLimit(r)
	if err != nil {
		return err
	}
	after, ok := ParseLogCursor(r.URL.Query().Get("after"))
	if !ok {
		return httpx.BadRequest("invalid_cursor", "after must look like <attempt>.<sequence>.")
	}
	lines, err := h.svc.Logs(r.Context(), auth.UserFrom(r.Context()).ID, jobID, after, limit)
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
