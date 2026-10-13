// Package githubtest is an in-memory GitHub API for tests. It checks App
// JWT signatures with the App's public key, so tests exercise real signing.
package githubtest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/gitprovider/github"
)

// Server fakes the parts of github.com and api.github.com ForgeDeck uses.
type Server struct {
	*httptest.Server
	AppID        int64
	ClientID     string
	ClientSecret string
	Key          *rsa.PrivateKey

	mu            sync.Mutex
	installations map[int64]string              // installation -> account login
	repositories  map[int64][]github.Repository // installation -> repos
	codes         map[string][]int64            // OAuth code -> installations the user can access
	tokensIssued  int
	tokens        map[string]issuedToken
}

type issuedToken struct {
	installation int64
	scope        TokenScope
}

// New starts a fake GitHub that is closed when the test ends.
func New(t testing.TB) *Server {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		AppID: 4242, ClientID: "Iv1.test", ClientSecret: "client-secret", Key: key,
		installations: map[int64]string{},
		repositories:  map[int64][]github.Repository{},
		codes:         map[string][]int64{},
		tokens:        map[string]issuedToken{},
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.route))
	t.Cleanup(s.Close)
	return s
}

// Config returns a client config pointing at this fake.
func (s *Server) Config() github.Config {
	return github.Config{
		AppID: s.AppID, Slug: "forgedeck-test", ClientID: s.ClientID, ClientSecret: s.ClientSecret,
		PrivateKey: s.Key, WebURL: s.URL, APIURL: s.URL,
	}
}

// PrivateKeyPEM returns the App key as PKCS#1 PEM, as GitHub hands it out.
func (s *Server) PrivateKeyPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(s.Key)})
}

// AddInstallation registers an installation on account with repos.
func (s *Server) AddInstallation(id int64, account string, repos ...github.Repository) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.installations[id] = account
	s.repositories[id] = repos
}

// AddOAuthCode makes code exchangeable for a user who can access installations.
func (s *Server) AddOAuthCode(code string, installations ...int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codes[code] = installations
}

// TokenScope is what an installation token was restricted to when minted.
// Zero values mean the token covers the whole installation.
type TokenScope struct {
	RepositoryIDs []int64           `json:"repository_ids"`
	Permissions   map[string]string `json:"permissions"`
}

// TokenScope returns how token was scoped, and whether the fake issued it.
func (s *Server) TokenScope(token string) (TokenScope, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	issued, ok := s.tokens[token]
	return issued.scope, ok
}

// TokensIssued counts installation tokens minted so far.
func (s *Server) TokensIssued() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokensIssued
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := r.URL.Path
	switch {
	case r.Method == http.MethodPost && path == "/login/oauth/access_token":
		s.exchange(w, r)
	case r.Method == http.MethodGet && path == "/user/installations":
		s.userInstallations(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/app/installations/"):
		s.withApp(w, r, func(id int64) {
			writeJSON(w, map[string]any{"id": id, "account": map[string]string{"login": s.installations[id]}})
		})
	case r.Method == http.MethodPost && strings.HasPrefix(path, "/app/installations/") && strings.HasSuffix(path, "/access_tokens"):
		s.withApp(w, r, func(id int64) {
			s.tokensIssued++
			var scope TokenScope
			_ = json.NewDecoder(r.Body).Decode(&scope)
			token := fmt.Sprintf("ghs_%d_%d", id, s.tokensIssued)
			s.tokens[token] = issuedToken{installation: id, scope: scope}
			writeJSON(w, map[string]any{"token": token, "expires_at": time.Now().Add(time.Hour)})
		})
	case r.Method == http.MethodGet && path == "/installation/repositories":
		s.installationRepos(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) withApp(w http.ResponseWriter, r *http.Request, fn func(id int64)) {
	if !s.validJWT(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")) {
		http.Error(w, `{"message":"bad JWT"}`, http.StatusUnauthorized)
		return
	}
	idPart := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/app/installations/"), "/access_tokens")
	id, err := strconv.ParseInt(idPart, 10, 64)
	if _, ok := s.installations[id]; err != nil || !ok {
		http.NotFound(w, r)
		return
	}
	fn(id)
}

func (s *Server) validJWT(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return false
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(&s.Key.PublicKey, crypto.SHA256, digest[:], signature) != nil {
		return false
	}
	payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		Iss string `json:"iss"`
		Exp int64  `json:"exp"`
	}
	return json.Unmarshal(payload, &claims) == nil &&
		claims.Iss == strconv.FormatInt(s.AppID, 10) && time.Unix(claims.Exp, 0).After(time.Now())
}

func (s *Server) exchange(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	if r.Form.Get("client_id") != s.ClientID || r.Form.Get("client_secret") != s.ClientSecret {
		writeJSON(w, map[string]string{"error_description": "bad client credentials"})
		return
	}
	if _, ok := s.codes[r.Form.Get("code")]; !ok {
		writeJSON(w, map[string]string{"error_description": "bad verification code"})
		return
	}
	writeJSON(w, map[string]string{"access_token": "ghu_" + r.Form.Get("code")})
}

func (s *Server) userInstallations(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ghu_")
	ids, ok := s.codes[code]
	if !ok {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		return
	}
	list := make([]map[string]any, len(ids))
	for i, id := range ids {
		list[i] = map[string]any{"id": id}
	}
	writeJSON(w, map[string]any{"installations": list})
}

func (s *Server) installationRepos(w http.ResponseWriter, r *http.Request) {
	issued, ok := s.tokens[strings.TrimPrefix(r.Header.Get("Authorization"), "token ")]
	if !ok {
		http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		return
	}
	id := issued.installation
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	repos := s.repositories[id]
	start := min((page-1)*perPage, len(repos))
	end := min(start+perPage, len(repos))
	writeJSON(w, map[string]any{"repositories": repos[start:end]})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
