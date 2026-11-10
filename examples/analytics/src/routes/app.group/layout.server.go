// Package app holds the pages that need a signed-in user.
package app

import (
	"net/http"
	"net/url"

	"github.com/Unfield/Tern/kit"

	core "analytics/internal/app"
	"analytics/internal/store"
)

type Shell struct {
	User  core.User    `json:"user"`
	Sites []store.Site `json:"sites"`
}

func LayoutLoad(ctx *kit.Ctx) (Shell, error) {
	u, ok := core.CurrentUser(ctx)
	if !ok {
		return Shell{}, kit.Redirect(http.StatusSeeOther, "/login?next="+url.QueryEscape(ctx.URL().RequestURI()))
	}
	sites, err := core.Store.Sites(ctx)
	if err != nil {
		return Shell{}, err
	}
	return Shell{User: u, Sites: sites}, nil
}
