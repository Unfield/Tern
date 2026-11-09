package routes

import (
	"github.com/Unfield/Tern/kit"

	"dashboard/internal/session"
)

type Root struct {
	User string `json:"user,omitempty"`
}

func LayoutLoad(ctx *kit.Ctx) (Root, error) {
	return Root{User: session.User(ctx)}, nil
}
