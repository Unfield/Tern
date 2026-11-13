package login

import (
	"net/http"
	"strings"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"shelf/internal/shelf"
)

// Access lets anyone in: this is where people sign in.
var Access = kit.Public

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

var Actions = kit.Actions{
	"default": kit.FormAction(func(ctx *kit.Ctx, in Credentials) (struct{}, error) {
		if _, err := shelf.Accounts.SignIn(ctx, in.Email, in.Password, in.Remember); err != nil {
			return struct{}{}, accounts.FormError(ctx, err)
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, safeNext(in.Next))
	}),
}

// safeNext keeps the redirect after signing in on this site: "next" comes
// from the URL, and anyone can make a link with one.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}
