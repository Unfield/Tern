package logout

import (
	"net/http"

	"github.com/Unfield/Tern/kit"

	"dashboard/internal/session"
)

var Actions = kit.Actions{
	"default": func(ctx *kit.Ctx) (any, error) {
		session.SignOut(ctx)
		return nil, kit.Redirect(http.StatusSeeOther, "/login")
	},
}
