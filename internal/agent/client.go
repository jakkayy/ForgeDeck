package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

const requestTimeout = 30 * time.Second

var (
	// ErrUnauthorized means the credential was rejected: the server was
	// removed from ForgeDeck. The Agent must stop and be re-enrolled.
	ErrUnauthorized = errors.New("control plane rejected the agent credential")
	// ErrLeaseLost means a job attempt no longer belongs to this Agent.
	ErrLeaseLost = errors.New("job lease lost")
)

// APIError is a non-2xx response from the Control Plane.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("control plane returned %d %s: %s", e.Status, e.Code, e.Message)
}

// Client calls the Control Plane's Agent API.
type Client struct {
	baseURL    string
	credential string
	http       *http.Client
}

// NewClient returns a client for baseURL. credential may be empty before
// enrollment.
func NewClient(baseURL, credential string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		credential: credential,
		http:       &http.Client{},
	}
}

// Enroll exchanges a single-use token for this Agent's credential.
func (c *Client) Enroll(ctx context.Context, token, version string) (agentproto.EnrollResponse, error) {
	var resp agentproto.EnrollResponse
	err := c.do(ctx, requestTimeout, "/api/v1/agent/enroll", agentproto.EnrollRequest{
		Token: token, AgentVersion: version, ProtocolVersion: agentproto.ProtocolVersion,
	}, &resp)
	return resp, err
}

// do POSTs body as JSON and decodes a JSON response into out when non-nil.
// A response without a body (204) leaves out unchanged.
func (c *Client) do(ctx context.Context, timeout time.Duration, path string, body, out any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.credential != "" {
		req.Header.Set("Authorization", "Bearer "+c.credential)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 300 {
		return apiError(resp.StatusCode, data)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func apiError(status int, body []byte) error {
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	apiErr := &APIError{Status: status, Code: envelope.Error.Code, Message: envelope.Error.Message}
	switch {
	case status == http.StatusUnauthorized:
		return fmt.Errorf("%w: %w", ErrUnauthorized, apiErr)
	case status == http.StatusConflict && apiErr.Code == "lease_lost":
		return fmt.Errorf("%w: %w", ErrLeaseLost, apiErr)
	default:
		return apiErr
	}
}

// Heartbeat reports liveness and status.
func (c *Client) Heartbeat(ctx context.Context, req agentproto.HeartbeatRequest) (agentproto.HeartbeatResponse, error) {
	var resp agentproto.HeartbeatResponse
	err := c.do(ctx, requestTimeout, "/api/v1/agent/heartbeat", req, &resp)
	return resp, err
}
