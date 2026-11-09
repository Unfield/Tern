package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/Unfield/Tern/kit"

	"dashboard/internal/db"
	"dashboard/internal/session"
)

func main() {
	// Open the database at startup so a bad path fails fast.
	if _, err := db.Default(); err != nil {
		log.Fatal(err)
	}
	app, err := ternApp(
		kit.WithHook(session.Hook),
		kit.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, nil))),
	)
	if err != nil {
		log.Fatal(err)
	}
	if err := app.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
