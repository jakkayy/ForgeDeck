package overview

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

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
