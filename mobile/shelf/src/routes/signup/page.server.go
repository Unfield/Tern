package signup

import (
	"net/http"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"shelf/internal/shelf"
)

// Access lets anyone in: this is where accounts are made.
var Access = kit.Public

type NewAccount struct {
	Name     string `form:"name" validate:"required,max=80"`
	Email    string `form:"email" validate:"required,email,max=254"`
	Password string `form:"password" validate:"required,min=10,max=200"`
}

var Actions = kit.Actions{
	"default": kit.FormAction(func(ctx *kit.Ctx, in NewAccount) (struct{}, error) {
		// SignUp creates the account and signs it in.
		if _, err := shelf.Accounts.SignUp(ctx, in.Email, in.Name, in.Password); err != nil {
			// A taken email, say, becomes a message on the form.
			return struct{}{}, accounts.FormError(ctx, err)
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/")
	}),
}
