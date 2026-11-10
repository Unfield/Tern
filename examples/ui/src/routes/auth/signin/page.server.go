// Package signin shows Tern UI's SignIn block with a password action.
package signin

import (
	"net/http"

	"github.com/Unfield/Tern/kit"
)

type Credentials struct {
	Email    string `form:"email" validate:"required,email"`
	Password string `form:"password" validate:"required"`
	Remember bool   `form:"remember"`
	Next     string `form:"next"`
}

type Page struct {
	Error string `json:"error"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	return Page{Error: ctx.URL().Query().Get("error")}, nil
}

var Actions = kit.Actions{
	"default": kit.FormAction(func(ctx *kit.Ctx, in Credentials) (struct{}, error) {
		// A real app checks a password hash here.
		if in.Email != "ada@example.com" || in.Password != "correct horse" {
			return struct{}{}, kit.FormError("Wrong email or password.")
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/auth/consent")
	}),
}
