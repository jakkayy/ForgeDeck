// Package gitlab talks to GitLab with a project or group access token.
package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var (
	// ErrUnauthorized means GitLab rejected the token.
	ErrUnauthorized = errors.New("GitLab rejected the access token")
	// ErrNotFound means the resource does not exist or the token cannot see it.
	ErrNotFound = errors.New("not found on GitLab")
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient returns a client for baseURL; httpClient should come from safehttp (user-supplied URL)
func NewClient(httpClient *http.Client, baseURL, token string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: httpClient}
}

type TokenInfo struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
	Active bool     `json:"active"`
}

type User struct {
	Username string `json:"username"`
}

type Project struct {
	ID                int64  `json:"id"`
	PathWithNamespace string `json:"path_with_namespace"`
	DefaultBranch     string `json:"default_branch"`
	Visibility        string `json:"visibility"`
}

func (c *Client) Token(ctx context.Context) (TokenInfo, error) {
	var info TokenInfo
	_, err := c.get(ctx, "/api/v4/personal_access_tokens/self", &info)
	return info, err
}

func (c *Client) CurrentUser(ctx context.Context) (User, error) {
	var user User
	_, err := c.get(ctx, "/api/v4/user", &user)
	return user, err
}

func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	var all []Project
	for page := "1"; page != ""; {
		var batch []Project
		header, err := c.get(ctx, "/api/v4/projects?membership=true&min_access_level=30&simple=true&per_page=100&page="+page, &batch)
		if err != nil {
			return nil, err
		}
		all = append(all, batch...)
		page = header.Get("X-Next-Page")
	}
	return all, nil
}

func (c *Client) CreateProjectHook(ctx context.Context, projectID int64, hookURL, token string) (int64, error) {
	body, _ := json.Marshal(map[string]any{
		"url": hookURL, "token": token, "push_events": true, "enable_ssl_verification": true,
	})
	var hook struct {
		ID int64 `json:"id"`
	}
	path := "/api/v4/projects/" + strconv.FormatInt(projectID, 10) + "/hooks"
	if _, err := c.send(ctx, http.MethodPost, path, body, &hook); err != nil {
		return 0, err
	}
	return hook.ID, nil
}

// DeleteProjectHook removes a webhook; an already-gone hook is fine
func (c *Client) DeleteProjectHook(ctx context.Context, projectID, hookID int64) error {
	path := "/api/v4/projects/" + strconv.FormatInt(projectID, 10) + "/hooks/" + strconv.FormatInt(hookID, 10)
	_, err := c.send(ctx, http.MethodDelete, path, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

type Commit struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

func (c *Client) Commit(ctx context.Context, projectID int64, ref string) (Commit, error) {
	var commit Commit
	_, err := c.get(ctx, "/api/v4/projects/"+strconv.FormatInt(projectID, 10)+"/repository/commits/"+url.PathEscape(ref), &commit)
	return commit, err
}

func (c *Client) get(ctx context.Context, path string, out any) (http.Header, error) {
	return c.send(ctx, http.MethodGet, path, nil, out)
}

func (c *Client) send(ctx context.Context, method, path string, payload []byte, out any) (http.Header, error) {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitLab request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNotFound
	case resp.StatusCode >= 300:
		return nil, fmt.Errorf("GitLab returned %s", strconv.Itoa(resp.StatusCode))
	}
	if out == nil {
		return resp.Header, nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return nil, fmt.Errorf("GitLab returned unexpected data: %w", err)
	}
	return resp.Header, nil
}
