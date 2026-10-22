// Package worker is the shared client side of the job protocol for Agents and Runners
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
)

const requestTimeout = 30 * time.Second

var (
	// ErrUnauthorized means the credential was rejected; the worker must stop
	ErrUnauthorized = errors.New("control plane rejected the credential")
	// ErrLeaseLost means a job attempt no longer belongs to this worker.
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

// API base paths for each kind of worker.
const (
	AgentAPI  = "/api/v1/agent"
	RunnerAPI = "/api/v1/runner"
)

// Client calls one worker API (AgentAPI or RunnerAPI) of the Control Plane.
type Client struct {
	baseURL string
	http    *http.Client

	mu         sync.RWMutex
	credential string
}

// NewClient returns a client for the API at controlPlaneURL+api
func NewClient(controlPlaneURL, api, credential string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(controlPlaneURL, "/") + api,
		credential: credential,
		http:       &http.Client{},
	}
}

// SetCredential replaces the credential for later requests (Agents do this on rotation)
func (c *Client) SetCredential(credential string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.credential = credential
}

func (c *Client) bearer() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.credential
}

// Post sends body to an endpoint below the client's API, such as "/enroll", and decodes the JSON response into out when non-nil
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, requestTimeout, path, body, out)
}

// do POSTs body as JSON and decodes a JSON response into out when non-nil
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
	if credential := c.bearer(); credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
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
	err := c.do(ctx, requestTimeout, "/heartbeat", req, &resp)
	return resp, err
}

// Claim waits up to wait for the next job; returns (nil, nil) when none arrived
func (c *Client) Claim(ctx context.Context, wait time.Duration) (*agentproto.Job, error) {
	seconds := int(wait / time.Second)
	var job agentproto.Job
	if err := c.do(ctx, wait+requestTimeout, "/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &seconds}, &job); err != nil {
		return nil, err
	}
	if job.ID == uuid.Nil {
		return nil, nil
	}
	return &job, nil
}

// RenewLease extends the lease on a job attempt.
func (c *Client) RenewLease(ctx context.Context, job *agentproto.Job) error {
	return c.do(ctx, requestTimeout, "/jobs/"+job.ID.String()+"/heartbeat",
		agentproto.LeaseRequest{Attempt: job.Attempt}, nil)
}

// SendEvents appends log events to a job attempt.
func (c *Client) SendEvents(ctx context.Context, job *agentproto.Job, events []agentproto.LogEvent) error {
	return c.do(ctx, requestTimeout, "/jobs/"+job.ID.String()+"/events",
		agentproto.EventsRequest{Attempt: job.Attempt, Events: events}, nil)
}

// Complete reports the outcome of a job attempt.
func (c *Client) Complete(ctx context.Context, job *agentproto.Job, req agentproto.CompleteRequest) error {
	req.Attempt = job.Attempt
	return c.do(ctx, requestTimeout, "/jobs/"+job.ID.String()+"/complete", req, nil)
}
