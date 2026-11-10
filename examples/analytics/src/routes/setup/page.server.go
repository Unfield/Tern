// Package setup creates the first account on a new install.
package setup

import (
	"net/http"

	"github.com/Unfield/Tern/kit"
	"golang.org/x/crypto/bcrypt"

	"analytics/internal/app"
)

type Owner struct {
	Name     string `form:"name" validate:"required,max=80"`
	Email    string `form:"email" validate:"required,email"`
	Password string `form:"password" validate:"required,min=12"`
}

// Load only shows the page while there are no accounts.
func Load(ctx *kit.Ctx) (struct{}, error) {
	n, err := app.Store.CountUsers(ctx)
	if err != nil {
		return struct{}{}, err
	}
	if n > 0 {
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/login")
	}
	return struct{}{}, nil
}

var Actions = kit.Actions{
	"default": kit.FormAction(func(ctx *kit.Ctx, in Owner) (struct{}, error) {
		n, err := app.Store.CountUsers(ctx)
		if err != nil {
			return struct{}{}, err
		}
		if n > 0 {
			return struct{}{}, kit.FormError("This install already has an owner. Sign in instead.")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			return struct{}{}, err
		}
		u, err := app.Store.CreateUser(ctx, in.Name, in.Email, hash)
		if err != nil {
			return struct{}{}, err
		}
		if err := app.SignIn(ctx, u.ID); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/sites")
	}),
}
