// Package migrations holds the app's database migrations, applied in
// order when the app starts (see the migrate package).
package migrations

import "embed"

// FS holds the migration files.
//
//go:embed *.sql
var FS embed.FS
