// Package logout ends the session.
package logout

import (
	"net/http"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/plugins/sessions"
)

var Actions = kit.Actions{
	"default": func(ctx *kit.Ctx) (any, error) {
		sessions.Get(ctx).Clear()
		return nil, kit.Redirect(http.StatusSeeOther, "/login")
	},
}
