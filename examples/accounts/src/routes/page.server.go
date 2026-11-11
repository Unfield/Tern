package routes

import (
	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"
)

type Home struct {
	User *accounts.User `json:"user"`
}

func Load(ctx *kit.Ctx) (Home, error) {
	return Home{User: accounts.Current(ctx)}, nil
}
