package httpx

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// sseWriteWindow is how long each write may take. Streams outlive the
// server's normal write timeout, so the deadline is extended per write.
const sseWriteWindow = 30 * time.Second

// SSE writes a text/event-stream response.
type SSE struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

// NewSSE sends the stream headers. Proxies are asked not to buffer.
func NewSSE(w http.ResponseWriter) *SSE {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	return &SSE{w: w, rc: http.NewResponseController(w)}
}

// Event writes one event with JSON data. id may be empty.
func (s *SSE) Event(name, id string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "event: %s\n", name)
	if id != "" {
		fmt.Fprintf(&b, "id: %s\n", id)
	}
	fmt.Fprintf(&b, "data: %s\n\n", payload)
	return s.write(b.String())
}

// Comment writes a comment line, used as a keepalive through proxies.
func (s *SSE) Comment(text string) error {
	return s.write(": " + text + "\n\n")
}

// Flush sends buffered events to the client.
func (s *SSE) Flush() error {
	return s.rc.Flush()
}

func (s *SSE) write(chunk string) error {
	_ = s.rc.SetWriteDeadline(time.Now().Add(sseWriteWindow))
	_, err := s.w.Write([]byte(chunk))
	return err
}
