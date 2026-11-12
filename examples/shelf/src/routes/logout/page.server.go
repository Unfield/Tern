package logout

import (
	"net/http"

	"github.com/Unfield/Tern/kit"

	"shelf/internal/shelf"
)

// Signing out changes state, so it's a form post, not a link.
var Actions = kit.Actions{
	"default": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		if err := shelf.Accounts.SignOut(ctx); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/login")
	}),
}
