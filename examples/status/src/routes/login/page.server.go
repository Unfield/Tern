// Package login signs administrators in.
package login

import (
	"net/http"
	"strings"

	"github.com/Unfield/Tern/kit"

	"status/internal/app"
)

func Load(ctx *kit.Ctx) (struct{}, error) {
	if _, ok := app.CurrentAdmin(ctx); ok {
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/admin")
	}
	if n, err := app.Store.UserCount(ctx); err == nil && n == 0 {
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/setup")
	}
	return struct{}{}, nil
}

type Invalid struct {
	Username string `json:"username"`
	Error    string `json:"error"`
}

var Actions = kit.Actions{
	"default": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		form, err := ctx.Form()
		if err != nil {
			return struct{}{}, err
		}
		username := strings.TrimSpace(form.Get("username"))
		u, err := app.Store.UserByName(ctx, username)
		if err != nil || !app.CheckPassword(u.PasswordHash, form.Get("password")) {
			return struct{}{}, kit.Fail(http.StatusUnauthorized, Invalid{Username: username, Error: "Wrong username or password."})
		}
		if err := app.SignIn(ctx, app.Admin{ID: u.ID, Username: u.Username}); err != nil {
			return struct{}{}, err
		}
		next := ctx.Request.URL.Query().Get("next")
		if !strings.HasPrefix(next, "/admin") {
			next = "/admin"
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, next)
	}),
}
