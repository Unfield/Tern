// Package form is a sign-up form whose rules live in Go: kit.FormAction
// checks them on the server, and Tern UI's Form checks the same rules in
// the browser from ./$forms.
package form

import (
	"strings"

	"github.com/Unfield/Tern/kit"
)

// SignUp is the form. Its tags are the rules for both sides.
type SignUp struct {
	Name     string `form:"name" validate:"required,max=80" label:"Name"`
	Email    string `form:"email" validate:"required,email" label:"Email"`
	Password string `form:"password" validate:"required,min=10" label:"Password"`
	Confirm  string `form:"confirm" validate:"required,same=password" label:"Confirm password"`
	Role     string `form:"role" validate:"required,oneof=dev|ops|other" label:"Role" message:"Choose a role."`
	Website  string `form:"website" validate:"url" label:"Website"`
	Terms    bool   `form:"terms" validate:"required" message:"Accept the terms to continue."`
}

// Welcome is what a successful sign-up returns as the page's form.
type Welcome struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

var Actions = kit.Actions{
	"default": kit.FormAction(func(ctx *kit.Ctx, in SignUp) (Welcome, error) {
		// Rules that need the server, such as a database, run here.
		if strings.EqualFold(in.Email, "taken@example.com") {
			return Welcome{}, kit.FieldError("email", "That email already has an account.")
		}
		return Welcome{Name: in.Name, Email: in.Email}, nil
	}),
}
