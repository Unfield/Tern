// Package events streams every check to the admin pages.
package events

import (
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/realtime"

	"status/internal/app"
)

func GET(ctx *kit.Ctx) error {
	if _, ok := app.CurrentAdmin(ctx); !ok {
		return kit.Error(401, "Sign in to follow checks")
	}
	return realtime.Stream(ctx, app.Hub, "monitors")
}
