package httpx

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// UUIDParam parses a chi URL parameter; a malformed ID is reported as not found
func UUIDParam(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.Nil, NotFound("Resource not found.")
	}
	return id, nil
}
