// Package audit records who did what, and lets project admins read it back
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// Non-human actors
const (
	ActorSystem  = "system"
	ActorAgent   = "agent"
	ActorRunner  = "runner"
	ActorWebhook = "webhook"
)

// Event is one thing that happened
type Event struct {
	ProjectID   *uuid.UUID
	ActorUserID *uuid.UUID
	Actor       string
	// action is "<noun>.<verb>", such as "secret.set" or "deployment.start"
	Action     string
	TargetType string
	TargetID   string
	TargetName string
	Metadata   map[string]any
}

// Recorder writes audit events
type Recorder struct {
	queries *store.Queries
	logger  *slog.Logger
}

// NewRecorder returns a Recorder
func NewRecorder(queries *store.Queries, logger *slog.Logger) *Recorder {
	return &Recorder{queries: queries, logger: logger}
}

// Record stores e
func (r *Recorder) Record(ctx context.Context, e Event) {
	if err := r.write(ctx, e); err != nil {
		r.logger.ErrorContext(ctx, "audit event not recorded", "action", e.Action, "target", e.TargetID, "error", err)
	}
}

// RecordIn stores e in the caller's transaction so it commits or rolls back together
func (r *Recorder) RecordIn(ctx context.Context, tx pgx.Tx, e Event) {
	sp, err := tx.Begin(ctx)
	if err == nil {
		err = r.writeWith(ctx, r.queries.WithTx(sp), e)
		if err != nil {
			_ = sp.Rollback(ctx)
		} else {
			err = sp.Commit(ctx)
		}
	}
	if err != nil {
		r.logger.ErrorContext(ctx, "audit event not recorded", "action", e.Action, "target", e.TargetID, "error", err)
	}
}

func (r *Recorder) write(ctx context.Context, e Event) error {
	return r.writeWith(ctx, r.queries, e)
}

func (r *Recorder) writeWith(ctx context.Context, q *store.Queries, e Event) error {
	actor := e.Actor
	if e.ActorUserID != nil {
		user, err := q.GetUserByID(ctx, *e.ActorUserID)
		if err != nil {
			return fmt.Errorf("look up actor: %w", err)
		}
		actor = user.Email
	}
	if actor == "" {
		actor = ActorSystem
	}
	metadata, err := json.Marshal(sanitize(e.Metadata))
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}
	return q.InsertAuditLog(ctx, store.InsertAuditLogParams{
		ProjectID: e.ProjectID, ActorUserID: e.ActorUserID, Actor: actor,
		Action: e.Action, TargetType: e.TargetType, TargetID: optional(e.TargetID), TargetName: optional(e.TargetName),
		Metadata: metadata, RequestID: optional(httpx.RequestIDFrom(ctx)),
	})
}

// sensitiveKeys are fragments of metadata keys whose values are never kept
var sensitiveKeys = []string{"secret", "password", "token", "credential", "key", "value"}

// sanitize masks metadata values under credential-like keys
func sanitize(metadata map[string]any) map[string]any {
	out := make(map[string]any, len(metadata))
	for key, value := range metadata {
		lower := strings.ToLower(key)
		masked := false
		for _, fragment := range sensitiveKeys {
			if strings.Contains(lower, fragment) {
				masked = true
				break
			}
		}
		if masked {
			out[key] = "[redacted]"
		} else {
			out[key] = value
		}
	}
	return out
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Entry is the API view of an audit event
type Entry struct {
	ID         uuid.UUID       `json:"id"`
	Actor      string          `json:"actor"`
	Action     string          `json:"action"`
	TargetType string          `json:"target_type"`
	TargetID   *string         `json:"target_id"`
	TargetName *string         `json:"target_name"`
	Metadata   json.RawMessage `json:"metadata"`
	RequestID  *string         `json:"request_id"`
	CreatedAt  time.Time       `json:"created_at"`
}

// Reader lists audit events for project admins
type Reader struct {
	queries *store.Queries
	authz   *authz.Authorizer
}

// NewReader returns a Reader
func NewReader(queries *store.Queries, authorizer *authz.Authorizer) *Reader {
	return &Reader{queries: queries, authz: authorizer}
}

// List returns a project's events, newest first, and the cursor of the next page (nil on the last)
func (r *Reader) List(ctx context.Context, userID, projectID uuid.UUID, action string, cursor string, limit int32) ([]Entry, *string, error) {
	if _, err := r.authz.Require(ctx, userID, projectID, authz.ViewAudit); err != nil {
		return nil, nil, err
	}
	before, beforeID, ok := httpx.DecodeTimeCursor(cursor)
	if !ok {
		return nil, nil, httpx.BadRequest("invalid_cursor", "cursor is not valid.")
	}
	params := store.ListAuditLogsParams{ProjectID: &projectID, BeforeCreatedAt: before, BeforeID: beforeID, PageLimit: limit}
	if action != "" {
		params.Action = &action
	}
	rows, err := r.queries.ListAuditLogs(ctx, params)
	if err != nil {
		return nil, nil, fmt.Errorf("list audit logs: %w", err)
	}
	entries := make([]Entry, len(rows))
	for i, row := range rows {
		entries[i] = Entry{
			ID: row.ID, Actor: row.Actor, Action: row.Action, TargetType: row.TargetType, TargetID: row.TargetID,
			TargetName: row.TargetName, Metadata: row.Metadata, RequestID: row.RequestID, CreatedAt: row.CreatedAt,
		}
	}
	if int32(len(rows)) < limit { //nolint:gosec // len is bounded by limit
		return entries, nil, nil
	}
	last := rows[len(rows)-1]
	next := httpx.EncodeTimeCursor(last.CreatedAt, last.ID)
	return entries, &next, nil
}
