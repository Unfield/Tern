package login

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/Unfield/Tern/kit"

	"dashboard/internal/db"
	"dashboard/internal/session"
)

func Load(ctx *kit.Ctx) (struct{}, error) {
	if session.User(ctx) != "" {
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/")
	}
	return struct{}{}, nil
}

type Invalid struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

var Actions = kit.Actions{
	"default": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		form, err := ctx.Form()
		if err != nil {
			return struct{}{}, err
		}
		name := strings.TrimSpace(form.Get("name"))
		if name == "" || utf8.RuneCountInString(name) > 40 {
			return struct{}{}, kit.Fail(http.StatusBadRequest, Invalid{Name: name, Error: ctx.T("login.invalid", map[string]any{"max": 40})})
		}
		store, err := db.Default()
		if err != nil {
			return struct{}{}, err
		}
		if err := store.SeedIfEmpty(ctx, name); err != nil {
			return struct{}{}, err
		}
		session.SignIn(ctx, name)
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/")
	}),
}
