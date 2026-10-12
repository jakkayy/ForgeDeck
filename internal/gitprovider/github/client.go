// Package github talks to GitHub as the ForgeDeck GitHub App: it verifies
// installations, mints short-lived installation tokens, and lists the
// repositories an installation grants.
package github

import (
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

// Config identifies the GitHub App. URLs default to github.com and can point
// at GitHub Enterprise Server.
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

// Client is a GitHub App API client. Installation tokens are cached until
// shortly before they expire.
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

// NewClient returns a Client for the App described by cfg.
func NewClient(cfg Config, now func() time.Time) *Client {
	cfg.WebURL = strings.TrimRight(cfg.WebURL, "/")
	cfg.APIURL = strings.TrimRight(cfg.APIURL, "/")
	return &Client{cfg: cfg, http: &http.Client{Timeout: 15 * time.Second}, now: now, tokens: map[int64]installationToken{}}
}

// WebURL is the GitHub web address, e.g. https://github.com.
func (c *Client) WebURL() string { return c.cfg.WebURL }

// InstallURL is where a user installs the App; GitHub sends them back to the
// App's setup URL with installation_id, code, and this state.
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

// UserCanAccessInstallation exchanges the OAuth code GitHub returns after
// installation and reports whether that user can access installationID.
// This stops someone from claiming another account's installation by
// editing installation_id in the callback URL.
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

// InstallationToken returns a token scoped to the installation, reusing a
// cached one while it has more than five minutes left.
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
	if err := c.send(ctx, http.MethodPost, c.cfg.APIURL+path, "Bearer "+jwt, nil, &resp); err != nil {
		return "", fmt.Errorf("create installation token: %w", err)
	}
	c.mu.Lock()
	c.tokens[installationID] = installationToken{value: resp.Token, expiresAt: resp.ExpiresAt}
	c.mu.Unlock()
	return resp.Token, nil
}

func (c *Client) exchangeCode(ctx context.Context, code string) (string, error) {
	form := url.Values{"client_id": {c.cfg.ClientID}, "client_secret": {c.cfg.ClientSecret}, "code": {code}}
	var resp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error_description"`
	}
	if err := c.send(ctx, http.MethodPost, c.cfg.WebURL+"/login/oauth/access_token", "", strings.NewReader(form.Encode()), &resp); err != nil {
		return "", fmt.Errorf("exchange OAuth code: %w", err)
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("exchange OAuth code: %s", resp.Error)
	}
	return resp.AccessToken, nil
}

func (c *Client) get(ctx context.Context, path, authorization string, out any) error {
	return c.send(ctx, http.MethodGet, c.cfg.APIURL+path, authorization, nil, out)
}

func (c *Client) send(ctx context.Context, method, target, authorization string, form io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, target, form)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode >= 300:
		return fmt.Errorf("GitHub returned %d: %s", resp.StatusCode, truncate(body))
	}
	return json.Unmarshal(body, out)
}

func truncate(b []byte) string {
	const limit = 200
	if len(b) > limit {
		return string(b[:limit]) + "…"
	}
	return string(b)
}
