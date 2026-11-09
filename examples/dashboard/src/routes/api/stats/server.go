// Package stats serves GET /api/stats as JSON.
package stats

import (
	"github.com/Unfield/Tern/kit"

	"dashboard/internal/app"
)

func GET(ctx *kit.Ctx) error {
	user, store, err := app.User(ctx)
	if err != nil {
		return err
	}
	stats, err := store.Stats(ctx, user)
	if err != nil {
		return err
	}
	return ctx.JSON(200, stats)
}
