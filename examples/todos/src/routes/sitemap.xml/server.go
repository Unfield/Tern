// Package sitemap serves /sitemap.xml, generated once by `tern build`.
package sitemap

import (
	"fmt"
	"strings"

	"github.com/Unfield/Tern/kit"
)

const Prerender = true

func GET(ctx *kit.Ctx) error {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, path := range []string{"/about", "/login"} {
		fmt.Fprintf(&b, "  <url><loc>https://todos.example.com%s</loc></url>\n", path)
	}
	b.WriteString("</urlset>\n")
	ctx.Writer().Header().Set("Content-Type", "application/xml")
	return ctx.Text(200, b.String())
}
