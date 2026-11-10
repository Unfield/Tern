// Package setup creates the first administrator.
package setup

import (
	"net/http"
	"strings"

	"github.com/Unfield/Tern/kit"

	"status/internal/app"
)

func Load(ctx *kit.Ctx) (struct{}, error) {
	if n, err := app.Store.UserCount(ctx); err != nil || n > 0 {
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/login")
	}
	return struct{}{}, nil
}

type Invalid struct {
	Username string            `json:"username"`
	Errors   map[string]string `json:"errors"`
}

var Actions = kit.Actions{
	"default": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		if n, err := app.Store.UserCount(ctx); err != nil || n > 0 {
			return struct{}{}, kit.Redirect(http.StatusSeeOther, "/login")
		}
		form, err := ctx.Form()
		if err != nil {
			return struct{}{}, err
		}
		username := strings.TrimSpace(form.Get("username"))
		password := form.Get("password")
		errs := map[string]string{}
		if len(username) < 3 || len(username) > 40 {
			errs["username"] = "Use 3 to 40 characters."
		}
		if len(password) < 10 {
			errs["password"] = "Use at least 10 characters."
		} else if password != form.Get("confirm") {
			errs["confirm"] = "The passwords don't match."
		}
		if len(errs) > 0 {
			return struct{}{}, kit.Fail(http.StatusBadRequest, Invalid{Username: username, Errors: errs})
		}
		hash, err := app.HashPassword(password)
		if err != nil {
			return struct{}{}, err
		}
		id, err := app.Store.CreateUser(ctx, username, hash)
		if err != nil {
			return struct{}{}, err
		}
		if err := app.SignIn(ctx, app.Admin{ID: id, Username: username}); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/admin")
	}),
}
