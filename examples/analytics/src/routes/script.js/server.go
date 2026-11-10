// Package script serves the tracking script that sites embed.
package script

import (

	"github.com/Unfield/Tern/kit"

	"analytics/internal/app"
)

func GET(ctx *kit.Ctx) error {
	w := ctx.Writer()
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	_, err := w.Write(app.Script)
	return err
}
