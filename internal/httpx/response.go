package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

// maxBodyBytes bounds JSON request bodies.
const maxBodyBytes = 1 << 20

// Error is an API error safe to show to clients.
type Error struct {
	Status  int
	Code    string
	Message string
	// Fields holds per-field validation messages, keyed by JSON field name.
	Fields map[string]string
}

func (e *Error) Error() string { return fmt.Sprintf("%d %s: %s", e.Status, e.Code, e.Message) }

// Common API errors.
func BadRequest(code, message string) *Error {
	return &Error{Status: http.StatusBadRequest, Code: code, Message: message}
}

func Unauthorized(message string) *Error {
	return &Error{Status: http.StatusUnauthorized, Code: "unauthorized", Message: message}
}

func Forbidden(message string) *Error {
	return &Error{Status: http.StatusForbidden, Code: "forbidden", Message: message}
}

func NotFound(message string) *Error {
	return &Error{Status: http.StatusNotFound, Code: "not_found", Message: message}
}

func Conflict(code, message string) *Error {
	return &Error{Status: http.StatusConflict, Code: code, Message: message}
}

// Invalid reports field validation failures.
func Invalid(fields map[string]string) *Error {
	return &Error{
		Status:  http.StatusUnprocessableEntity,
		Code:    "validation_failed",
		Message: "Some fields are invalid.",
		Fields:  fields,
	}
}

var errInternal = &Error{
	Status:  http.StatusInternalServerError,
	Code:    "internal_error",
	Message: "Something went wrong. Try again or check the server logs.",
}

// HandlerFunc is an HTTP handler that returns an error instead of writing it.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Handle adapts a HandlerFunc, writing any returned error as an API error.
func Handle(fn HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			WriteError(w, r, err)
		}
	}
}

// WriteJSON writes v as a JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		return nil
	}
	return json.NewEncoder(w).Encode(v)
}

// WriteError writes err in the standard error envelope. apperr.Error maps to
// its status, validate.Errors become 422 responses; other errors that are not *Error are hidden from the client
// and recorded on the access log line.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *Error
	var appErr *apperr.Error
	var fieldErrs validate.Errors
	switch {
	case errors.As(err, &apiErr):
	case errors.As(err, &appErr):
		apiErr = &Error{Status: statusFor(appErr.Kind), Code: appErr.Code, Message: appErr.Message}
	case errors.As(err, &fieldErrs):
		apiErr = Invalid(fieldErrs)
	default:
		recordError(r.Context(), err)
		apiErr = errInternal
	}
	body := struct {
		Error errorBody `json:"error"`
	}{errorBody{
		Code:      apiErr.Code,
		Message:   apiErr.Message,
		Fields:    apiErr.Fields,
		RequestID: RequestIDFrom(r.Context()),
	}}
	_ = WriteJSON(w, apiErr.Status, body)
}

func statusFor(kind apperr.Kind) int {
	switch kind {
	case apperr.KindNotFound:
		return http.StatusNotFound
	case apperr.KindForbidden:
		return http.StatusForbidden
	case apperr.KindConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

type errorBody struct {
	Code      string            `json:"code"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
	RequestID string            `json:"request_id,omitempty"`
}

// DecodeJSON reads a bounded JSON body into dst, rejecting unknown fields.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return &Error{Status: http.StatusRequestEntityTooLarge, Code: "body_too_large", Message: "Request body is too large."}
		}
		return BadRequest("invalid_json", "Request body is not valid JSON for this endpoint.")
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return BadRequest("invalid_json", "Request body must contain a single JSON object.")
	}
	return nil
}

// Recover turns a panic into a 500 response and records it for the access log.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v)
				}
				WriteError(w, r, fmt.Errorf("panic: %v", v))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// errorSlot lets WriteError hand an internal error to AccessLog.
type errorSlot struct{ err error }

type errorSlotKey struct{}

func withErrorSlot(ctx context.Context) (context.Context, *errorSlot) {
	slot := &errorSlot{}
	return context.WithValue(ctx, errorSlotKey{}, slot), slot
}

func recordError(ctx context.Context, err error) {
	if slot, ok := ctx.Value(errorSlotKey{}).(*errorSlot); ok {
		slot.err = err
	}
}

// TooManyRequests sets Retry-After and returns a 429 error for the handler to return.
func TooManyRequests(w http.ResponseWriter, retryAfter time.Duration) *Error {
	seconds := int(math.Ceil(retryAfter.Seconds()))
	w.Header().Set("Retry-After", strconv.Itoa(max(seconds, 1)))
	return &Error{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "Too many attempts. Wait a moment and try again."}
}

// ClientIP returns the address of the client that made the request. It is
// the TCP peer unless TrustedProxies is in front of the handler and the peer
// is one of the trusted proxies, in which case it is the address that proxy
// reported (see TrustedProxies).
func ClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(clientIPKey{}).(string); ok {
		return ip
	}
	return peerIP(r)
}

func peerIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
