// Package logout signs the administrator out.
package logout

import (
	"net/http"

	"github.com/Unfield/Tern/kit"

	"status/internal/app"
)

var Actions = kit.Actions{
	"default": func(ctx *kit.Ctx) (any, error) {
		app.SignOut(ctx)
		return nil, kit.Redirect(http.StatusSeeOther, "/")
	},
}
