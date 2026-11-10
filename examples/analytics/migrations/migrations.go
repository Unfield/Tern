// Package migrations holds the database's schema changes.
package migrations

import "embed"

// FS is the migrations, applied in order at startup.
//
//go:embed *.sql
var FS embed.FS
