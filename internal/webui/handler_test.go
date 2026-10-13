package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

var site = fstest.MapFS{
	"index.html":         {Data: []byte("<html>app</html>")},
	"favicon.svg":        {Data: []byte("<svg/>")},
	"assets/app-1a2b.js": {Data: []byte("console.log(1)")},
}

func get(t *testing.T, fsys fstest.MapFS, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	Handler(fsys).ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestHandlerServesFilesWithCachePolicy(t *testing.T) {
	cases := []struct {
		path, body, cache string
	}{
		{"/", "<html>app</html>", "no-cache"},
		{"/assets/app-1a2b.js", "console.log(1)", "public, max-age=31536000, immutable"},
		{"/favicon.svg", "<svg/>", "public, max-age=3600"},
		{"/servers/3f2a", "<html>app</html>", "no-cache"},
		{"/projects/1/members", "<html>app</html>", "no-cache"},
	}
	for _, tc := range cases {
		rec := get(t, site, http.MethodGet, tc.path)
		if rec.Code != http.StatusOK || rec.Body.String() != tc.body || rec.Header().Get("Cache-Control") != tc.cache {
			t.Errorf("%s: %d %q cache=%q", tc.path, rec.Code, rec.Body, rec.Header().Get("Cache-Control"))
		}
	}
}

func TestHandlerNotFound(t *testing.T) {
	for _, target := range []string{"/assets/missing.js", "/robots.txt", "/../../etc/passwd.txt"} {
		if rec := get(t, site, http.MethodGet, target); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d", target, rec.Code)
		}
	}
	if rec := get(t, fstest.MapFS{}, http.MethodGet, "/servers"); rec.Code != http.StatusNotFound {
		t.Errorf("unbuilt UI: status %d", rec.Code)
	}
}

func TestHandlerRejectsWrites(t *testing.T) {
	rec := get(t, site, http.MethodPost, "/")
	if rec.Code != http.StatusMethodNotAllowed || !strings.Contains(rec.Header().Get("Allow"), "GET") {
		t.Fatalf("status %d", rec.Code)
	}
}
