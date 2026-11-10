// Package event receives page views from /script.js.
package event

import (
	"github.com/Unfield/Tern/kit"

	"analytics/internal/app"
)

func POST(ctx *kit.Ctx) error    { return app.Collect(ctx) }
func OPTIONS(ctx *kit.Ctx) error { return app.Collect(ctx) }
