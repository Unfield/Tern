package code

import (
	"net/http"
	"strings"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"accounts/internal/auth"
)

type Page struct {
	Next string `json:"next"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	if !auth.Acc.AwaitingCode(ctx) {
		return Page{}, kit.Redirect(http.StatusSeeOther, "/login")
	}
	return Page{Next: safeNext(ctx.URL().Query().Get("next"))}, nil
}

func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/account"
	}
	return next
}

type Code struct {
	Code string `form:"code" validate:"required,max=20" label:"Code"`
}

var Actions = kit.Actions{
	"default": kit.FormAction(func(ctx *kit.Ctx, in Code) (struct{}, error) {
		if _, err := auth.Acc.CompleteSignIn(ctx, in.Code); err != nil {
			return struct{}{}, accounts.FormError(ctx, err)
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, safeNext(ctx.URL().Query().Get("next")))
	}),
}
