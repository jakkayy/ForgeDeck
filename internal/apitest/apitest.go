// Package apitest runs the full Control Plane router against an isolated
// test database so handler tests exercise real routing, middleware, and SQL.
package apitest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/app"
	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/keyring"
	"github.com/jakkayy/ForgeDeck/internal/safehttp"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/testdb"
)

// Password is used for every user created by Env.CreateUser.
const Password = "test-password-123"

// Env is a running API server backed by a fresh database.
type Env struct {
	t      testing.TB
	Pool   *pgxpool.Pool
	Server *httptest.Server
}

// Option adjusts the configuration of the API under test.
type Option func(*config.Config)

// New starts the API for one test.
func New(t testing.TB, options ...Option) *Env {
	t.Helper()
	pool := testdb.New(t)
	keys, err := keyring.Parse("test:AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=")
	if err != nil {
		t.Fatal(err)
	}
	// Fakes for Git providers listen on 127.0.0.1, which production blocks.
	cfg := config.Config{CookieSecure: false, Keyring: keys, Outbound: safehttp.Policy{AllowLoopback: true}}
	for _, option := range options {
		option(&cfg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(app.NewRouter(ctx, cfg, pool, slog.New(slog.DiscardHandler)))
	t.Cleanup(func() {
		cancel()
		server.Close()
	})
	return &Env{t: t, Pool: pool, Server: server}
}

// CreateUser stores a user with Password and returns its ID.
func (e *Env) CreateUser(email string) uuid.UUID {
	e.t.Helper()
	user, err := auth.NewService(store.New(e.Pool), time.Now).CreateUser(context.Background(), email, Password)
	if err != nil {
		e.t.Fatalf("apitest: create user %s: %v", email, err)
	}
	return user.ID
}

// AddMember gives userID a role in projectID directly in the database.
func (e *Env) AddMember(projectID, userID uuid.UUID, role string) {
	e.t.Helper()
	_, err := store.New(e.Pool).AddProjectMember(context.Background(), store.AddProjectMemberParams{
		ProjectID: projectID, UserID: userID, Role: role,
	})
	if err != nil {
		e.t.Fatalf("apitest: add member: %v", err)
	}
}

// Anonymous returns a client without a session.
func (e *Env) Anonymous() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{t: e.t, base: e.Server.URL, http: &http.Client{
		Jar: jar,
		// Return redirects to the test instead of following them.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}

// SignIn logs in an existing user and returns a client carrying the session cookie.
func (e *Env) SignIn(email string) *Client {
	e.t.Helper()
	c := e.Anonymous()
	resp := c.Do(http.MethodPost, "/api/v1/auth/login", map[string]string{"email": email, "password": Password})
	resp.RequireStatus(http.StatusOK)
	return c
}

// NewUser creates a user and returns a signed-in client and the user's ID.
func (e *Env) NewUser(email string) (*Client, uuid.UUID) {
	e.t.Helper()
	id := e.CreateUser(email)
	return e.SignIn(email), id
}

// Client is an HTTP client for the test server.
type Client struct {
	t      testing.TB
	base   string
	http   *http.Client
	bearer string
}

// Response is a fully read HTTP response.
type Response struct {
	t      testing.TB
	Status int
	Header http.Header
	Body   []byte
}

// Do sends a request with body encoded as JSON (nil for no body).
func (c *Client) Do(method, path string, body any) *Response {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			c.t.Fatalf("apitest: encode body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.base+path, reader)
	if err != nil {
		c.t.Fatalf("apitest: new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("apitest: %s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatalf("apitest: read body: %v", err)
	}
	return &Response{t: c.t, Status: resp.StatusCode, Header: resp.Header, Body: data}
}

// IssueEnrollmentToken creates an enrollment token for a server named name.
func (c *Client) IssueEnrollmentToken(projectID uuid.UUID, name string) string {
	c.t.Helper()
	var token struct {
		Token string `json:"token"`
	}
	c.Do(http.MethodPost, "/api/v1/projects/"+projectID.String()+"/servers/enrollment-tokens",
		map[string]string{"server_name": name}).RequireStatus(http.StatusCreated).Decode(&token)
	return token.Token
}

// Agent is an enrolled Agent with a client that authenticates as it.
type Agent struct {
	agentproto.EnrollResponse
	*Client
}

// EnrollAgent registers a server named name in projectID, using owner to
// issue the token, and returns a client authenticated as its Agent.
func (e *Env) EnrollAgent(owner *Client, projectID uuid.UUID, name string) Agent {
	e.t.Helper()
	token := owner.IssueEnrollmentToken(projectID, name)
	var enrolled agentproto.EnrollResponse
	e.Anonymous().Do(http.MethodPost, "/api/v1/agent/enroll", agentproto.EnrollRequest{
		Token: token, AgentVersion: "test", ProtocolVersion: agentproto.ProtocolVersion,
	}).RequireStatus(http.StatusCreated).Decode(&enrolled)

	client := e.Anonymous()
	client.bearer = enrolled.Credential
	return Agent{EnrollResponse: enrolled, Client: client}
}

// ReportContainers sends a snapshot of running containers with the given
// IDs (12+ lowercase hex characters) and requires it to be accepted.
func (a Agent) ReportContainers(t testing.TB, ids ...string) {
	t.Helper()
	containers := make([]agentproto.Container, len(ids))
	for i, id := range ids {
		containers[i] = agentproto.Container{ID: id, Name: "c" + id[:6], Image: "example:1", State: "running"}
	}
	var resp agentproto.SnapshotResponse
	a.Do(http.MethodPost, "/api/v1/agent/containers/snapshot", agentproto.ContainerSnapshot{
		ObservedAt: time.Now(), Containers: containers,
	}).RequireStatus(http.StatusOK).Decode(&resp)
	if !resp.Accepted {
		t.Fatal("apitest: snapshot not accepted")
	}
}

// CreateProject creates a project owned by the client's user and returns its ID.
func (c *Client) CreateProject(name string) uuid.UUID {
	c.t.Helper()
	var p struct {
		ID uuid.UUID `json:"id"`
	}
	c.Do(http.MethodPost, "/api/v1/projects", map[string]string{"name": name}).RequireStatus(http.StatusCreated).Decode(&p)
	return p.ID
}

// RequireStatus fails the test unless the response has the given status.
func (r *Response) RequireStatus(want int) *Response {
	r.t.Helper()
	if r.Status != want {
		r.t.Fatalf("status %d, want %d: %s", r.Status, want, r.Body)
	}
	return r
}

// Decode unmarshals the JSON body into dst.
func (r *Response) Decode(dst any) {
	r.t.Helper()
	if err := json.Unmarshal(r.Body, dst); err != nil {
		r.t.Fatalf("apitest: decode %s: %v", r.Body, err)
	}
}

// ErrorCode returns error.code from an error envelope.
func (r *Response) ErrorCode() string {
	r.t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	r.Decode(&env)
	return env.Error.Code
}
