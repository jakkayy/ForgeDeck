// Package webui embeds the built dashboard (apps/web) into the API binary.
// `make web` copies the Vite build into ./dist before `go build`; without it
// the binary still builds and serves only the API.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// FS returns the dashboard files rooted at the build output.
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // "dist" is embedded above, so this cannot fail
	}
	return sub
}

// Built reports whether the dashboard was built into this binary.
func Built() bool {
	_, err := fs.Stat(FS(), "index.html")
	return err == nil
}
