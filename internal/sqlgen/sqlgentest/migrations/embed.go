// Package migrations is the schema the sqlgen tests type queries against.
package migrations

import "embed"

// FS holds the migrations.
//
//go:embed *.sql
var FS embed.FS
