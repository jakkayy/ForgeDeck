package httpx

import (
	"net/http"
	"strconv"
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

// ParsePage reads limit and cursor, applying defaults and bounds.
func ParsePage(r *http.Request) (Page, error) {
	page := Page{Limit: defaultPageLimit}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err := strconv.ParseInt(raw, 10, 32)
		if err != nil || limit < 1 {
			return Page{}, BadRequest("invalid_limit", "limit must be a positive integer.")
		}
		page.Limit = int32(min(limit, maxPageLimit)) //nolint:gosec // clamped to maxPageLimit
	}
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
