// Package github talks to GitHub as the ForgeDeck GitHub App.
package github

import (
	"bytes"
	"context"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config identifies the GitHub App; URLs can point at GitHub Enterprise Server
type Config struct {
	AppID        int64
	Slug         string
	ClientID     string
	ClientSecret string
	PrivateKey   *rsa.PrivateKey
	WebURL       string // e.g. https://github.com
	APIURL       string // e.g. https://api.github.com
}

// ErrNotFound means GitHub returned 404 for the resource.
var ErrNotFound = errors.New("not found on GitHub")

// Client is a GitHub App API client; installation tokens are cached until near expiry
type Client struct {
	cfg  Config
	http *http.Client
	now  func() time.Time

	mu     sync.Mutex
	tokens map[int64]installationToken
}

type installationToken struct {
	value     string
	expiresAt time.Time
}

func NewClient(cfg Config, now func() time.Time) *Client {
	cfg.WebURL = strings.TrimRight(cfg.WebURL, "/")
	cfg.APIURL = strings.TrimRight(cfg.APIURL, "/")
	return &Client{cfg: cfg, http: &http.Client{Timeout: 15 * time.Second}, now: now, tokens: map[int64]installationToken{}}
}

func (c *Client) WebURL() string { return c.cfg.WebURL }

// InstallURL is where a user installs the App
func (c *Client) InstallURL(state string) string {
	return fmt.Sprintf("%s/apps/%s/installations/new?state=%s", c.cfg.WebURL, url.PathEscape(c.cfg.Slug), url.QueryEscape(state))
}

// Installation is an App installation on a user or organization account.
type Installation struct {
	ID      int64 `json:"id"`
	Account struct {
		Login string `json:"login"`
	} `json:"account"`
}

// Repository is a repository visible to an installation.
type Repository struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
	Private       bool   `json:"private"`
}

// UserCanAccessInstallation checks the OAuth code, so installation_id in the callback cannot be forged
func (c *Client) UserCanAccessInstallation(ctx context.Context, code string, installationID int64) (bool, error) {
	userToken, err := c.exchangeCode(ctx, code)
	if err != nil {
		return false, err
	}
	var page struct {
		Installations []Installation `json:"installations"`
	}
	for p := 1; ; p++ {
		path := "/user/installations?per_page=100&page=" + strconv.Itoa(p)
		if err := c.get(ctx, path, "Bearer "+userToken, &page); err != nil {
			return false, fmt.Errorf("list user installations: %w", err)
		}
		if slices.ContainsFunc(page.Installations, func(i Installation) bool { return i.ID == installationID }) {
			return true, nil
		}
		if len(page.Installations) < 100 {
			return false, nil
		}
	}
}

// Installation looks up an installation as the App.
func (c *Client) Installation(ctx context.Context, installationID int64) (Installation, error) {
	jwt, err := appJWT(c.cfg.AppID, c.cfg.PrivateKey, c.now())
	if err != nil {
		return Installation{}, err
	}
	var inst Installation
	err = c.get(ctx, "/app/installations/"+strconv.FormatInt(installationID, 10), "Bearer "+jwt, &inst)
	return inst, err
}

// Repositories lists every repository the installation grants.
func (c *Client) Repositories(ctx context.Context, installationID int64) ([]Repository, error) {
	token, err := c.InstallationToken(ctx, installationID)
	if err != nil {
		return nil, err
	}
	var all []Repository
	for p := 1; ; p++ {
		var page struct {
			Repositories []Repository `json:"repositories"`
		}
		if err := c.get(ctx, "/installation/repositories?per_page=100&page="+strconv.Itoa(p), "token "+token, &page); err != nil {
			return nil, fmt.Errorf("list repositories: %w", err)
		}
		all = append(all, page.Repositories...)
		if len(page.Repositories) < 100 {
			return all, nil
		}
	}
}

// Commit is a commit's identity and message.
type Commit struct {
	SHA     string
	Message string
}

// Commit resolves ref (a branch, tag, or SHA) in the repository fullName.
func (c *Client) Commit(ctx context.Context, installationID int64, fullName, ref string) (Commit, error) {
	token, err := c.InstallationToken(ctx, installationID)
	if err != nil {
		return Commit{}, err
	}
	var resp struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	}
	if err := c.get(ctx, "/repos/"+fullName+"/commits/"+url.PathEscape(ref), "token "+token, &resp); err != nil {
		return Commit{}, fmt.Errorf("get commit: %w", err)
	}
	return Commit{SHA: resp.SHA, Message: resp.Commit.Message}, nil
}

// InstallationToken returns a cached token while it has more than five minutes left
func (c *Client) InstallationToken(ctx context.Context, installationID int64) (string, error) {
	c.mu.Lock()
	cached, ok := c.tokens[installationID]
	c.mu.Unlock()
	if ok && c.now().Before(cached.expiresAt.Add(-5*time.Minute)) {
		return cached.value, nil
	}

	jwt, err := appJWT(c.cfg.AppID, c.cfg.PrivateKey, c.now())
	if err != nil {
		return "", err
	}
	var resp struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	path := "/app/installations/" + strconv.FormatInt(installationID, 10) + "/access_tokens"
	if err := c.send(ctx, http.MethodPost, c.cfg.APIURL+path, "Bearer "+jwt, nil, "", &resp); err != nil {
		return "", fmt.Errorf("create installation token: %w", err)
	}
	c.mu.Lock()
	c.tokens[installationID] = installationToken{value: resp.Token, expiresAt: resp.ExpiresAt}
	c.mu.Unlock()
	return resp.Token, nil
}

// RepositoryReadToken mints an uncached, read-only token for one repository
func (c *Client) RepositoryReadToken(ctx context.Context, installationID, repositoryID int64) (string, error) {
	jwt, err := appJWT(c.cfg.AppID, c.cfg.PrivateKey, c.now())
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]any{
		"repository_ids": []int64{repositoryID},
		"permissions":    map[string]string{"contents": "read"},
	})
	if err != nil {
		return "", err
	}
	var resp struct {
		Token string `json:"token"`
	}
	path := "/app/installations/" + strconv.FormatInt(installationID, 10) + "/access_tokens"
	if err := c.send(ctx, http.MethodPost, c.cfg.APIURL+path, "Bearer "+jwt, bytes.NewReader(body), "application/json", &resp); err != nil {
		return "", fmt.Errorf("create repository token: %w", err)
	}
	return resp.Token, nil
}

func (c *Client) exchangeCode(ctx context.Context, code string) (string, error) {
	form := url.Values{"client_id": {c.cfg.ClientID}, "client_secret": {c.cfg.ClientSecret}, "code": {code}}
	var resp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error_description"`
	}
	if err := c.send(ctx, http.MethodPost, c.cfg.WebURL+"/login/oauth/access_token", "",
		strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", &resp); err != nil {
		return "", fmt.Errorf("exchange OAuth code: %w", err)
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("exchange OAuth code: %s", resp.Error)
	}
	return resp.AccessToken, nil
}

func (c *Client) get(ctx context.Context, path, authorization string, out any) error {
	return c.send(ctx, http.MethodGet, c.cfg.APIURL+path, authorization, nil, "", out)
}

func (c *Client) send(ctx context.Context, method, target, authorization string, body io.Reader, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if contentType == "application/x-www-form-urlencoded" {
		// GitHub's OAuth endpoint answers form posts with a form unless asked for JSON.
		req.Header.Set("Accept", "application/json")
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode >= 300:
		return fmt.Errorf("GitHub returned %d: %s", resp.StatusCode, truncate(data))
	}
	return json.Unmarshal(data, out)
}

func truncate(b []byte) string {
	const limit = 200
	if len(b) > limit {
		return string(b[:limit]) + "…"
	}
	return string(b)
}
