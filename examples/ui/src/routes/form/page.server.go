// Package form validates a sign-up form in Go and shows the errors with
// Tern UI's Field.
package form

import (
	"net/mail"
	"strings"

	"github.com/Unfield/Tern/kit"
)

// Result is what the page gets back as form.
type Result struct {
	Values map[string]string `json:"values"`
	Errors map[string]string `json:"errors,omitempty"`
	Done   bool              `json:"done,omitempty"`
}

var Actions = kit.Actions{
	"default": kit.Action(func(ctx *kit.Ctx) (Result, error) {
		f, err := ctx.Form()
		if err != nil {
			return Result{}, err
		}
		r := Result{Values: map[string]string{"name": f.Get("name"), "email": f.Get("email"), "role": f.Get("role")}, Errors: map[string]string{}}
		if strings.TrimSpace(r.Values["name"]) == "" {
			r.Errors["name"] = "Enter your name."
		}
		if _, err := mail.ParseAddress(r.Values["email"]); err != nil {
			r.Errors["email"] = "Enter an email address, like ada@example.com."
		}
		if r.Values["role"] == "" {
			r.Errors["role"] = "Choose a role."
		}
		if f.Get("terms") != "on" {
			r.Errors["terms"] = "Accept the terms to continue."
		}
		if len(r.Errors) > 0 {
			return Result{}, kit.Fail(400, r)
		}
		return Result{Values: r.Values, Done: true}, nil
	}),
}
