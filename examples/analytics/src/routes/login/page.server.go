// Package login signs in with an email and password.
package login

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Unfield/Tern/kit"
	"golang.org/x/crypto/bcrypt"

	"analytics/internal/app"
	"analytics/internal/store"
)

type Credentials struct {
	Email    string `form:"email" validate:"required,email"`
	Password string `form:"password" validate:"required"`
	Next     string `form:"next"`
}

type Page struct {
	Next string `json:"next"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	if _, ok := app.CurrentUser(ctx); ok {
		return Page{}, kit.Redirect(http.StatusSeeOther, "/sites")
	}
	return Page{Next: ctx.URL().Query().Get("next")}, nil
}

// A hash to compare against when the email is unknown, so both cases take
// as long and nobody can tell which emails have accounts.
var dummy, _ = bcrypt.GenerateFromPassword([]byte("dummy password"), bcrypt.DefaultCost)

var Actions = kit.Actions{
	"default": kit.FormAction(func(ctx *kit.Ctx, in Credentials) (struct{}, error) {
		u, err := app.Store.UserByEmail(ctx, in.Email)
		hash := dummy
		if err == nil {
			hash = u.Hash()
		} else if !errors.Is(err, store.ErrNotFound) {
			return struct{}{}, err
		}
		if bcrypt.CompareHashAndPassword(hash, []byte(in.Password)) != nil || err != nil {
			return struct{}{}, kit.FormError("Wrong email or password.")
		}
		if err := app.SignIn(ctx, u.ID); err != nil {
			return struct{}{}, err
		}
		next := in.Next
		if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
			next = "/sites"
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, next)
	}),
}
