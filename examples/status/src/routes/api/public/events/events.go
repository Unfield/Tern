// Package events streams public monitors' checks to the status page.
package events

import (
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/realtime"

	"status/internal/app"
)

func GET(ctx *kit.Ctx) error { return realtime.Stream(ctx, app.Hub, "public") }
