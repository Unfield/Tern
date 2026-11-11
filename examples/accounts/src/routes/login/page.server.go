package login

import (
	"net/http"
	"strings"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"accounts/internal/auth"
)

type Credentials struct {
	Email    string `form:"email" validate:"required,email"`
	Password string `form:"password" validate:"required"`
	Remember bool   `form:"remember"`
	Next     string `form:"next"`
}

type Page struct {
	Next string `json:"next"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	next := safeNext(ctx.URL().Query().Get("next"))
	if accounts.Current(ctx) != nil {
		return Page{}, kit.Redirect(http.StatusSeeOther, next)
	}
	return Page{Next: next}, nil
}

// safeNext keeps redirects on this site.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/account"
	}
	return next
}

var Actions = kit.Actions{
	"default": kit.FormAction(func(ctx *kit.Ctx, in Credentials) (struct{}, error) {
		if _, err := auth.Acc.SignIn(ctx, in.Email, in.Password, in.Remember); err != nil {
			return struct{}{}, accounts.FormError(ctx, err)
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, safeNext(in.Next))
	}),
}
