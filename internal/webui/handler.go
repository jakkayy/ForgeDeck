package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Handler serves the dashboard; hashed assets are cached, index.html is revalidated, SPA routes fall back
func Handler(fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}

		if isFile(fsys, name) {
			w.Header().Set("Cache-Control", cacheControl(name))
			http.ServeFileFS(w, r, fsys, name)
			return
		}
		if path.Ext(name) != "" || !isFile(fsys, "index.html") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", cacheControl("index.html"))
		http.ServeFileFS(w, r, fsys, "index.html")
	})
}

func isFile(fsys fs.FS, name string) bool {
	info, err := fs.Stat(fsys, name)
	return err == nil && !info.IsDir()
}

func cacheControl(name string) string {
	switch {
	case name == "index.html":
		return "no-cache"
	case strings.HasPrefix(name, "assets/"):
		return "public, max-age=31536000, immutable"
	default:
		return "public, max-age=3600"
	}
}
