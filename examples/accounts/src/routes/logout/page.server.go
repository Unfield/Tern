package logout

import (
	"net/http"

	"github.com/Unfield/Tern/kit"

	"accounts/internal/auth"
)

var Actions = kit.Actions{
	"default": func(ctx *kit.Ctx) (any, error) {
		if err := auth.Acc.SignOut(ctx); err != nil {
			return nil, err
		}
		return nil, kit.Redirect(http.StatusSeeOther, "/login")
	},
}
