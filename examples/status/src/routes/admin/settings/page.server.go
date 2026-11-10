// Package settings configures the status page and the administrator.
package settings

import (
	"net/http"
	"strings"

	"github.com/Unfield/Tern/kit"

	"status/internal/app"
)

type Page struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	return Page{Title: app.Store.Setting(ctx, "title", "Status"), Description: app.Store.Setting(ctx, "description", "")}, nil
}

type Result struct {
	Saved  string            `json:"saved"`
	Errors map[string]string `json:"errors"`
}

var Actions = kit.Actions{
	"page": kit.Action(func(ctx *kit.Ctx) (Result, error) {
		form, err := ctx.Form()
		if err != nil {
			return Result{}, err
		}
		title := strings.TrimSpace(form.Get("title"))
		if title == "" || len(title) > 60 {
			return Result{}, kit.Fail(http.StatusBadRequest, Result{Errors: map[string]string{"title": "Use 1 to 60 characters."}})
		}
		if err := app.Store.SetSetting(ctx, "title", title); err != nil {
			return Result{}, err
		}
		if err := app.Store.SetSetting(ctx, "description", strings.TrimSpace(form.Get("description"))); err != nil {
			return Result{}, err
		}
		return Result{Saved: "page"}, nil
	}),
	"password": kit.Action(func(ctx *kit.Ctx) (Result, error) {
		admin, err := app.RequireAdmin(ctx)
		if err != nil {
			return Result{}, err
		}
		form, err := ctx.Form()
		if err != nil {
			return Result{}, err
		}
		u, err := app.Store.UserByName(ctx, admin.Username)
		if err != nil {
			return Result{}, err
		}
		errs := map[string]string{}
		if !app.CheckPassword(u.PasswordHash, form.Get("current")) {
			errs["current"] = "That isn't your current password."
		}
		if len(form.Get("new")) < 10 {
			errs["new"] = "Use at least 10 characters."
		}
		if len(errs) > 0 {
			return Result{}, kit.Fail(http.StatusBadRequest, Result{Errors: errs})
		}
		hash, err := app.HashPassword(form.Get("new"))
		if err != nil {
			return Result{}, err
		}
		if err := app.Store.SetPassword(ctx, u.ID, hash); err != nil {
			return Result{}, err
		}
		return Result{Saved: "password"}, nil
	}),
}
