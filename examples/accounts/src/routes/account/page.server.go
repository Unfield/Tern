package account

import (
	"net/http"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"accounts/internal/auth"
)

type Page struct {
	User     accounts.User      `json:"user"`
	Sessions []accounts.Session `json:"sessions"`
	Passkeys []accounts.Passkey `json:"passkeys"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	u, err := auth.Acc.Require(ctx)
	if err != nil {
		return Page{}, err
	}
	sessions, err := auth.Acc.Sessions(ctx)
	if err != nil {
		return Page{}, err
	}
	passkeys, err := auth.Acc.Passkeys(ctx)
	if err != nil {
		return Page{}, err
	}
	return Page{User: *u, Sessions: sessions, Passkeys: passkeys}, nil
}

type ID struct {
	ID string `form:"id" validate:"required"`
}

type Name struct {
	Name string `form:"name" validate:"required,max=80"`
}

type Password struct {
	Current  string `form:"current" label:"Current password"`
	Password string `form:"password" validate:"required,min=10,max=200" label:"New password"`
}

type Done struct {
	Done string `json:"done"`
}

// do runs fn for a signed-in user, with the errors users cause as form
// errors.
func do(ctx *kit.Ctx, done string, fn func() error) (Done, error) {
	if _, err := auth.Acc.Require(ctx); err != nil {
		return Done{}, err
	}
	if err := fn(); err != nil {
		return Done{}, accounts.FormError(ctx, err)
	}
	return Done{Done: done}, nil
}

var Actions = kit.Actions{
	"name": kit.FormAction(func(ctx *kit.Ctx, in Name) (Done, error) {
		return do(ctx, "name", func() error { return auth.Acc.SetName(ctx, in.Name) })
	}),
	"password": kit.FormAction(func(ctx *kit.Ctx, in Password) (Done, error) {
		return do(ctx, "password", func() error { return auth.Acc.ChangePassword(ctx, in.Current, in.Password) })
	}),
	"verify": func(ctx *kit.Ctx) (any, error) {
		return do(ctx, "verify", func() error { return auth.Acc.SendVerification(ctx) })
	},
	"revoke": kit.FormAction(func(ctx *kit.Ctx, in ID) (Done, error) {
		return do(ctx, "", func() error { return auth.Acc.Revoke(ctx, in.ID) })
	}),
	"others": func(ctx *kit.Ctx) (any, error) {
		return do(ctx, "", func() error { return auth.Acc.SignOutOthers(ctx) })
	},
	"removePasskey": kit.FormAction(func(ctx *kit.Ctx, in ID) (Done, error) {
		return do(ctx, "", func() error { return auth.Acc.RemovePasskey(ctx, in.ID) })
	}),
	"delete": func(ctx *kit.Ctx) (any, error) {
		if _, err := do(ctx, "", func() error { return auth.Acc.DeleteAccount(ctx) }); err != nil {
			return nil, err
		}
		return nil, kit.Redirect(http.StatusSeeOther, "/")
	},
}
