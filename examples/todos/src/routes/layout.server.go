package routes

import (
	"github.com/Unfield/Tern/kit"

	"todos/internal/session"
)

// Root is available to every page as data.user.
type Root struct {
	User string `json:"user,omitempty"`
}

func LayoutLoad(ctx *kit.Ctx) (Root, error) {
	return Root{User: session.User(ctx)}, nil
}
