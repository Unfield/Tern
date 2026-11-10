// Package admin is the signed-in area: monitors, incidents, notifications
// and settings.
package admin

import (
	"github.com/Unfield/Tern/kit"

	"status/internal/app"
)

type Layout struct {
	Username string `json:"username"`
	Title    string `json:"title"`
	Notice   string `json:"notice"`
}

func LayoutLoad(ctx *kit.Ctx) (Layout, error) {
	a, err := app.RequireAdmin(ctx)
	if err != nil {
		return Layout{}, err
	}
	return Layout{Username: a.Username, Title: app.Store.Setting(ctx, "title", "Status"), Notice: app.TakeFlash(ctx)}, nil
}
