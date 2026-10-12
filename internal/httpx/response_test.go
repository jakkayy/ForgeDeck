package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

type errorEnvelope struct {
	Error struct {
		Code      string            `json:"code"`
		Message   string            `json:"message"`
		Fields    map[string]string `json:"fields"`
		RequestID string            `json:"request_id"`
	} `json:"error"`
}

func serve(handler http.Handler, req *http.Request) (*httptest.ResponseRecorder, *bytes.Buffer) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	rec := httptest.NewRecorder()
	RequestID(AccessLog(logger)(Recover(handler))).ServeHTTP(rec, req)
	return rec, &logs
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) errorEnvelope {
	t.Helper()
	var env errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("response is not an error envelope: %v (%q)", err, rec.Body.String())
	}
	return env
}

func TestHandleWritesAPIError(t *testing.T) {
	handler := Handle(func(http.ResponseWriter, *http.Request) error {
		return Invalid(map[string]string{"name": "Name is required."})
	})
	rec, _ := serve(handler, httptest.NewRequest(http.MethodPost, "/", nil))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", rec.Code)
	}
	env := decodeEnvelope(t, rec)
	if env.Error.Code != "validation_failed" || env.Error.Fields["name"] == "" || env.Error.RequestID == "" {
		t.Fatalf("unexpected envelope: %+v", env)
	}
}

func TestHandleMapsValidationErrors(t *testing.T) {
	handler := Handle(func(http.ResponseWriter, *http.Request) error {
		errs := validate.Errors{}
		errs.Add("email", "Email is required.")
		return fmt.Errorf("create user: %w", errs.Err())
	})
	rec, _ := serve(handler, httptest.NewRequest(http.MethodPost, "/", nil))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := decodeEnvelope(t, rec).Error.Fields["email"]; got != "Email is required." {
		t.Fatalf("fields.email = %q", got)
	}
}

func TestHandleMapsAppErrors(t *testing.T) {
	cases := map[*apperr.Error]int{
		apperr.NotFound("Project not found."):   http.StatusNotFound,
		apperr.Forbidden("Not allowed."):        http.StatusForbidden,
		apperr.Conflict("slug_taken", "Taken."): http.StatusConflict,
	}
	for appErr, want := range cases {
		handler := Handle(func(http.ResponseWriter, *http.Request) error {
			return fmt.Errorf("wrapped: %w", appErr)
		})
		rec, _ := serve(handler, httptest.NewRequest(http.MethodGet, "/", nil))
		env := decodeEnvelope(t, rec)
		if rec.Code != want || env.Error.Code != appErr.Code || env.Error.Message != appErr.Message {
			t.Errorf("%s: status %d envelope %+v", appErr.Code, rec.Code, env)
		}
	}
}

func TestHandleHidesInternalErrorAndLogsIt(t *testing.T) {
	handler := Handle(func(http.ResponseWriter, *http.Request) error {
		return errors.New("db password=hunter2 rejected")
	})
	rec, logs := serve(handler, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "hunter2") {
		t.Fatal("internal error leaked to client")
	}
	if decodeEnvelope(t, rec).Error.Code != "internal_error" {
		t.Fatal("wrong error code")
	}
	if !strings.Contains(logs.String(), "db password") {
		t.Fatalf("internal error not logged: %s", logs.String())
	}
}

func TestRecoverConvertsPanic(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })
	rec, logs := serve(handler, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError || decodeEnvelope(t, rec).Error.Code != "internal_error" {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logs.String(), "panic: boom") {
		t.Fatalf("panic not logged: %s", logs.String())
	}
}

func TestDecodeJSON(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	cases := []struct {
		name     string
		body     string
		wantCode string
	}{
		{"valid", `{"name":"api"}`, ""},
		{"unknown field", `{"name":"api","admin":true}`, "invalid_json"},
		{"trailing data", `{"name":"api"}{}`, "invalid_json"},
		{"malformed", `{"name":`, "invalid_json"},
		{"too large", `{"name":"` + strings.Repeat("a", maxBodyBytes) + `"}`, "body_too_large"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			var dst payload
			err := DecodeJSON(httptest.NewRecorder(), req, &dst)

			var apiErr *Error
			switch {
			case tc.wantCode == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantCode != "" && (!errors.As(err, &apiErr) || apiErr.Code != tc.wantCode):
				t.Fatalf("got %v, want code %s", err, tc.wantCode)
			}
		})
	}
}
