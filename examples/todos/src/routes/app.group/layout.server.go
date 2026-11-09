// Package app holds the pages that require signing in. The app.group
// directory groups them under one layout without adding to their URLs.
package app

import (
	"net/http"

	"github.com/Unfield/Tern/kit"

	"todos/internal/session"
)

type Account struct {
	SignedInAs string `json:"signedInAs"`
}

func LayoutLoad(ctx *kit.Ctx) (Account, error) {
	user := session.User(ctx)
	if user == "" {
		return Account{}, kit.Redirect(http.StatusSeeOther, "/login")
	}
	return Account{SignedInAs: user}, nil
}
