package httpx

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveWithRequestID(t *testing.T, incoming string) (header, inContext string) {
	t.Helper()
	handler := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		inContext = RequestIDFrom(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if incoming != "" {
		req.Header.Set(RequestIDHeader, incoming)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec.Header().Get(RequestIDHeader), inContext
}

func TestRequestIDGeneratesWhenMissing(t *testing.T) {
	header, inContext := serveWithRequestID(t, "")
	if !strings.HasPrefix(header, "req_") || header != inContext {
		t.Fatalf("header %q, context %q", header, inContext)
	}
}

func TestRequestIDReusesWellFormedIncomingID(t *testing.T) {
	header, inContext := serveWithRequestID(t, "abc-123")
	if header != "abc-123" || inContext != "abc-123" {
		t.Fatalf("header %q, context %q", header, inContext)
	}
}

func TestRequestIDReplacesMalformedIncomingID(t *testing.T) {
	header, _ := serveWithRequestID(t, "bad id\nwith newline")
	if !strings.HasPrefix(header, "req_") {
		t.Fatalf("malformed ID was kept: %q", header)
	}
}
