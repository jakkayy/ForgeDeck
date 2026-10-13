package httpx

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultPageLimit = 50
	maxPageLimit     = 100
)

// Page is a parsed ?limit=&cursor= pair. The cursor is opaque to clients;
// for offset-paginated lists it encodes the next offset.
type Page struct {
	Limit  int32
	Offset int32
}

// ParseLimit reads ?limit=, applying the default and maximum. Endpoints with
// keyset cursors use it directly and decode their own cursor.
func ParseLimit(r *http.Request) (int32, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return defaultPageLimit, nil
	}
	limit, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || limit < 1 {
		return 0, BadRequest("invalid_limit", "limit must be a positive integer.")
	}
	return int32(min(limit, maxPageLimit)), nil //nolint:gosec // clamped to maxPageLimit
}

// ParsePage reads limit and an offset cursor, applying defaults and bounds.
func ParsePage(r *http.Request) (Page, error) {
	limit, err := ParseLimit(r)
	if err != nil {
		return Page{}, err
	}
	page := Page{Limit: limit}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		offset, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || offset < 0 {
			return Page{}, BadRequest("invalid_cursor", "cursor is not valid.")
		}
		page.Offset = int32(offset) //nolint:gosec // ParseInt bitSize 32 bounds the value
	}
	return page, nil
}

// List is the response envelope for paginated collections.
type List[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

// NewList builds a List from rows fetched with page; a full page implies there
// may be more.
func NewList[T any](items []T, page Page) List[T] {
	list := List[T]{Items: items}
	if int32(len(items)) == page.Limit { //nolint:gosec // len is bounded by Limit
		next := strconv.Itoa(int(page.Offset + page.Limit))
		list.NextCursor = &next
	}
	return list
}

// EncodeTimeCursor makes the opaque keyset cursor for a list ordered by
// (created_at, id) newest first: the position after the last row of a page.
func EncodeTimeCursor(createdAt time.Time, id uuid.UUID) string {
	raw := strconv.FormatInt(createdAt.UnixMicro(), 10) + "_" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// DecodeTimeCursor parses a cursor from EncodeTimeCursor. An empty string is
// the first page (ok, with a nil position).
func DecodeTimeCursor(s string) (createdAt *time.Time, id *uuid.UUID, ok bool) {
	if s == "" {
		return nil, nil, true
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, nil, false
	}
	micros, idText, found := strings.Cut(string(raw), "_")
	us, errTime := strconv.ParseInt(micros, 10, 64)
	parsed, errID := uuid.Parse(idText)
	if !found || errTime != nil || errID != nil {
		return nil, nil, false
	}
	t := time.UnixMicro(us)
	return &t, &parsed, true
}
