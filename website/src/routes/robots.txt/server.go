// Package robots is /robots.txt, prerendered.
package robots

import (
	"net/http"

	"github.com/Unfield/Tern/kit"
)

const Prerender = true

func GET(ctx *kit.Ctx) error {
	ctx.SetHeader("Content-Type", "text/plain; charset=utf-8")
	return ctx.Text(http.StatusOK, "User-agent: *\nAllow: /\n\nSitemap: https://tern.land/sitemap.xml\n")
}
