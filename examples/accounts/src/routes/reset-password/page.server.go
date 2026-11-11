package resetpassword

import (
	"net/http"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"accounts/internal/auth"
)

type Page struct {
	Token string `json:"token"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	return Page{Token: ctx.URL().Query().Get("token")}, nil
}

type NewPassword struct {
	Token    string `form:"token" validate:"required"`
	Password string `form:"password" validate:"required,min=10,max=200"`
	Confirm  string `form:"confirm" validate:"required,same=password" label:"Confirm new password"`
}

var Actions = kit.Actions{
	"default": kit.FormAction(func(ctx *kit.Ctx, in NewPassword) (struct{}, error) {
		if _, err := auth.Acc.ResetPassword(ctx, in.Token, in.Password); err != nil {
			return struct{}{}, accounts.FormError(ctx, err)
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/account")
	}),
}
