package signup

import (
	"net/http"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"accounts/internal/auth"
)

type Account struct {
	Name     string `form:"name" validate:"required,max=80"`
	Email    string `form:"email" validate:"required,email,max=254"`
	Password string `form:"password" validate:"required,min=10,max=200"`
}

var Actions = kit.Actions{
	"default": kit.FormAction(func(ctx *kit.Ctx, in Account) (struct{}, error) {
		if _, err := auth.Acc.SignUp(ctx, in.Email, in.Name, in.Password); err != nil {
			return struct{}{}, accounts.FormError(ctx, err)
		}
		if err := auth.Acc.SendVerification(ctx); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/account")
	}),
}
