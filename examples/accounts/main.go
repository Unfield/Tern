package main

import (
	"log"

	"accounts/internal/auth"
)

func main() {
	acc, err := auth.Open()
	if err != nil {
		log.Fatal(err)
	}
	app, err := ternApp(auth.Plugin(acc))
	if err != nil {
		log.Fatal(err)
	}
	if err := app.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
