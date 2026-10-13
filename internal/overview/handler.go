package overview

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

// Handler serves the overview behind auth.RequireUser.
type Handler struct {
	svc *Service
}

// NewHandler returns the overview endpoint.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Routes mounts the overview endpoint on r.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/overview", httpx.Handle(h.get))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	summary, err := h.svc.Get(r.Context(), auth.UserFrom(r.Context()).ID)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, summary)
}
