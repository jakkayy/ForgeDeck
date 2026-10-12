// Package migrations embeds the SQL schema migrations into the binary.
package migrations

import "embed"

// FS holds every migration file, applied in version order by goose.
//
//go:embed *.sql
var FS embed.FS
