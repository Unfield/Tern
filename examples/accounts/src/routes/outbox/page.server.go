// Package outbox shows the emails the app sent, in place of an inbox.
package outbox

import (
	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"accounts/internal/auth"
)

type Page struct {
	Emails []accounts.Email `json:"emails"`
}

func Load(*kit.Ctx) (Page, error) {
	return Page{Emails: auth.Outbox()}, nil
}
