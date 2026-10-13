-- name: RecordWebhookEvent :one
-- Returns no row when the delivery was already recorded.
INSERT INTO webhook_events (provider, delivery_id, event_type, git_connection_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (provider, delivery_id) DO NOTHING
RETURNING id;

-- name: SetWebhookEventResult :exec
UPDATE webhook_events SET result = $2 WHERE id = $1;

-- name: DeleteWebhookEvent :exec
DELETE FROM webhook_events WHERE id = $1;
