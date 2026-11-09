package main

import (
	"log"

	"github.com/Unfield/Tern/kit"

	"todos/internal/session"
)

func main() {
	app, err := ternApp(kit.WithHook(session.Hook))
	if err != nil {
		log.Fatal(err)
	}
	if err := app.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
