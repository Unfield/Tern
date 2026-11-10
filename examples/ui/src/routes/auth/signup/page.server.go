// Package signup shows Tern UI's SignUp block, checked by kit.FormAction.
package signup

import (
	"net/http"

	"github.com/Unfield/Tern/kit"
)

type Account struct {
	Name     string `form:"name" validate:"required,max=80"`
	Email    string `form:"email" validate:"required,email"`
	Password string `form:"password" validate:"required,min=12"`
	Confirm  string `form:"confirm" validate:"required,same=password" label:"Confirm password"`
	Terms    bool   `form:"terms" validate:"required" message:"Agree to the terms to continue."`
}

var Actions = kit.Actions{
	"default": kit.FormAction(func(ctx *kit.Ctx, in Account) (struct{}, error) {
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/auth/signin")
	}),
}
