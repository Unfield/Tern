// Package docs embeds Tern's documentation, so tools such as tern mcp can
// serve the docs for the version in use.
package docs

import "embed"

// FS holds the Markdown documentation and the decision records.
//
//go:embed *.md adr/*.md
var FS embed.FS
