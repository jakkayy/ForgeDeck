package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type response struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

func call(t *testing.T, checks map[string]Check) (int, response) {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler(checks).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	var body response
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return rec.Code, body
}

func TestHandlerHealthy(t *testing.T) {
	code, body := call(t, map[string]Check{
		"database": func(context.Context) error { return nil },
	})
	if code != http.StatusOK || body.Status != "ok" || body.Checks["database"] != "ok" {
		t.Fatalf("code %d body %+v", code, body)
	}
}

func TestHandlerUnhealthyHidesError(t *testing.T) {
	code, body := call(t, map[string]Check{
		"database": func(context.Context) error { return errors.New("dial tcp 10.0.0.5:5432") },
	})
	if code != http.StatusServiceUnavailable || body.Status != "unavailable" || body.Checks["database"] != "failing" {
		t.Fatalf("code %d body %+v", code, body)
	}
}
