package main

import (
	"log"
	"net/http"

	"github.com/Unfield/Tern/kit"

	"todos/internal/session"
)

func main() {
	app, err := ternApp(
		kit.WithHook(session.Hook),
		// Routes with `var Access = kit.SignedIn` send signed-out visitors to
		// the login page.
		kit.WithSignedIn(func(ctx *kit.Ctx) error {
			if session.User(ctx) == "" {
				return kit.Redirect(http.StatusSeeOther, "/login")
			}
			return nil
		}),
	)
	if err != nil {
		log.Fatal(err)
	}
	if err := app.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
