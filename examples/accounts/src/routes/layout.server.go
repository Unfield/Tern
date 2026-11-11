package routes

import (
	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"
)

type Layout struct {
	User *accounts.User `json:"user"`
}

func LayoutLoad(ctx *kit.Ctx) (Layout, error) {
	return Layout{User: accounts.Current(ctx)}, nil
}
