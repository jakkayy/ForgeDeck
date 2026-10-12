// Package gitlab talks to GitLab (gitlab.com or self-hosted) with a project
// or group access token.
package gitlab

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// ErrUnauthorized means GitLab rejected the token.
var ErrUnauthorized = errors.New("GitLab rejected the access token")

// Client calls the GitLab REST API v4 with one token.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient returns a client for baseURL (e.g. https://gitlab.com). httpClient
// should come from safehttp, since baseURL is user supplied.
func NewClient(httpClient *http.Client, baseURL, token string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: httpClient}
}

// TokenInfo describes the access token in use.
type TokenInfo struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
	Active bool     `json:"active"`
}

// User is the account behind the token (a bot user for project tokens).
type User struct {
	Username string `json:"username"`
}

// Project is a repository the token can access.
type Project struct {
	ID                int64  `json:"id"`
	PathWithNamespace string `json:"path_with_namespace"`
	DefaultBranch     string `json:"default_branch"`
	Visibility        string `json:"visibility"`
}

// Token returns details of the token itself.
func (c *Client) Token(ctx context.Context) (TokenInfo, error) {
	var info TokenInfo
	_, err := c.get(ctx, "/api/v4/personal_access_tokens/self", &info)
	return info, err
}

// CurrentUser returns the user the token acts as.
func (c *Client) CurrentUser(ctx context.Context) (User, error) {
	var user User
	_, err := c.get(ctx, "/api/v4/user", &user)
	return user, err
}

// Projects lists projects where the token has at least Developer access.
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

func (c *Client) get(ctx context.Context, path string, out any) (http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
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
	case resp.StatusCode >= 300:
		return nil, fmt.Errorf("GitLab returned %s", strconv.Itoa(resp.StatusCode))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return nil, fmt.Errorf("GitLab returned unexpected data: %w", err)
	}
	return resp.Header, nil
}
