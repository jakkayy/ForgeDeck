package webhooks

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// delivery identifies one webhook delivery from a provider.
type delivery struct {
	provider     string
	id           string
	eventType    string
	connectionID *uuid.UUID
}

// process handles a verified, first-seen delivery and returns a short
// description of what was done, stored with the event for troubleshooting.
type process func(ctx context.Context) (string, error)

// handleOnce records the delivery and runs fn only if the provider has not
// sent it before. If fn fails, the record is removed so the provider's own
// retry is processed instead of being dropped as a duplicate.
func (h *Handler) handleOnce(w http.ResponseWriter, r *http.Request, d delivery, fn process) error {
	if d.id == "" {
		return httpx.BadRequest("missing_delivery_id", "The delivery ID header is missing.")
	}
	ctx := r.Context()
	eventID, err := h.queries.RecordWebhookEvent(ctx, store.RecordWebhookEventParams{
		Provider: d.provider, DeliveryID: d.id, EventType: d.eventType, GitConnectionID: d.connectionID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return httpx.WriteJSON(w, http.StatusOK, map[string]string{"result": "duplicate delivery ignored"})
	}
	if err != nil {
		return fmt.Errorf("record webhook event: %w", err)
	}

	result, err := fn(ctx)
	if err != nil {
		_ = h.queries.DeleteWebhookEvent(context.WithoutCancel(ctx), eventID)
		return err
	}
	if err := h.queries.SetWebhookEventResult(ctx, store.SetWebhookEventResultParams{ID: eventID, Result: &result}); err != nil {
		return fmt.Errorf("store webhook result: %w", err)
	}
	return httpx.WriteJSON(w, http.StatusAccepted, map[string]string{"result": result})
}
