package routes

import (
	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"shelf/internal/shelf"
)

type Layout struct {
	User *accounts.User `json:"user"`
	// Admin is whether the user may use the admin screens.
	Admin bool `json:"admin"`
	// Flags are the features this user gets.
	Flags map[string]bool `json:"flags"`
}

func LayoutLoad(ctx *kit.Ctx) (Layout, error) {
	u := accounts.Current(ctx)
	return Layout{User: u, Admin: shelf.IsAdmin(u), Flags: shelf.Flags.For(ctx, "search")}, nil
}
