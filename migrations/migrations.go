// Package migrations embeds the SQL migration files so the migrate command can
// run them without depending on the working directory.
package migrations

import "embed"

// FS holds every migration file in this directory.
//
//go:embed *.sql
var FS embed.FS
