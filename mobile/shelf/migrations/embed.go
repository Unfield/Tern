// Package migrations holds the database migrations, applied in order when
// the app starts.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
