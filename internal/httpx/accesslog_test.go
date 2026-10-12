package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAccessLogRecordsRequest(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	handler := RequestID(AccessLog(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("missing"))
	})))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/things?token=secret", nil)
	handler.ServeHTTP(httptest.NewRecorder(), req)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log line is not JSON: %v (%q)", err, buf.String())
	}
	checks := map[string]any{
		"level":  "WARN",
		"method": "GET",
		"path":   "/api/v1/things",
		"status": float64(404),
		"bytes":  float64(7),
	}
	for key, want := range checks {
		if entry[key] != want {
			t.Errorf("%s = %v, want %v", key, entry[key], want)
		}
	}
	if entry["request_id"] == "" {
		t.Error("request_id missing")
	}
	if bytes.Contains(buf.Bytes(), []byte("secret")) {
		t.Error("query string leaked into access log")
	}
}
