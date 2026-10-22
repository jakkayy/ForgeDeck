// Package gitlabtest is an in-memory GitLab API v4 for tests.
package gitlabtest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"sync"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/gitprovider/gitlab"
)

type Token struct {
	Username string
	Scopes   []string
	Projects []gitlab.Project
}

type Hook struct {
	ID        int64
	ProjectID int64
	URL       string
	Token     string
}

type Server struct {
	*httptest.Server
	mu      sync.Mutex
	tokens  map[string]Token
	hooks   map[int64]Hook
	commits map[string]gitlab.Commit // "<project ID>:<ref>"
	nextID  int64
}

func New(t testing.TB) *Server {
	t.Helper()
	s := &Server{tokens: map[string]Token{}, hooks: map[int64]Hook{}, commits: map[string]gitlab.Commit{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.route))
	t.Cleanup(s.Close)
	return s
}

func (s *Server) Hooks() []Hook {
	s.mu.Lock()
	defer s.mu.Unlock()
	hooks := make([]Hook, 0, len(s.hooks))
	for _, h := range s.hooks {
		hooks = append(hooks, h)
	}
	return hooks
}

func (s *Server) AddCommit(projectID int64, ref string, commit gitlab.Commit) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commits[strconv.FormatInt(projectID, 10)+":"+ref] = commit
}

func (s *Server) AddToken(value string, token Token) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[value] = token
}

func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	token, ok := s.tokens[r.Header.Get("PRIVATE-TOKEN")]
	s.mu.Unlock()
	if !ok {
		http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/personal_access_tokens/self":
		writeJSON(w, map[string]any{"name": "forgedeck", "scopes": token.Scopes, "active": true})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/user":
		writeJSON(w, map[string]string{"username": token.Username})
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/projects":
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		start := min((page-1)*perPage, len(token.Projects))
		end := min(start+perPage, len(token.Projects))
		if end < len(token.Projects) {
			w.Header().Set("X-Next-Page", strconv.Itoa(page+1))
		}
		writeJSON(w, token.Projects[start:end])
	case r.Method == http.MethodGet && commitPath.MatchString(r.URL.Path):
		match := commitPath.FindStringSubmatch(r.URL.Path)
		s.mu.Lock()
		commit, ok := s.commits[match[1]+":"+match[2]]
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, commit)
	case r.Method == http.MethodPost && hookPath.MatchString(r.URL.Path):
		var body struct {
			URL   string `json:"url"`
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		projectID, _ := strconv.ParseInt(hookPath.FindStringSubmatch(r.URL.Path)[1], 10, 64)
		s.mu.Lock()
		s.nextID++
		hook := Hook{ID: s.nextID, ProjectID: projectID, URL: body.URL, Token: body.Token}
		s.hooks[hook.ID] = hook
		s.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, map[string]int64{"id": hook.ID})
	case r.Method == http.MethodDelete && hookItemPath.MatchString(r.URL.Path):
		id, _ := strconv.ParseInt(hookItemPath.FindStringSubmatch(r.URL.Path)[2], 10, 64)
		s.mu.Lock()
		_, exists := s.hooks[id]
		delete(s.hooks, id)
		s.mu.Unlock()
		if !exists {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

var (
	hookPath     = regexp.MustCompile(`^/api/v4/projects/(\d+)/hooks$`)
	hookItemPath = regexp.MustCompile(`^/api/v4/projects/(\d+)/hooks/(\d+)$`)
	commitPath   = regexp.MustCompile(`^/api/v4/projects/(\d+)/repository/commits/(.+)$`)
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
