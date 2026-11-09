package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/plugins/compress"
	"github.com/Unfield/Tern/plugins/health"
	"github.com/Unfield/Tern/plugins/requestlog"
	"github.com/Unfield/Tern/plugins/secure"

	"dashboard/internal/db"
	"dashboard/internal/session"
)

func main() {
	// Open the database at startup so a bad path fails fast.
	store, err := db.Default()
	if err != nil {
		log.Fatal(err)
	}
	app, err := ternApp(
		kit.WithHook(session.Hook),
		kit.WithLogger(slog.New(slog.NewTextHandler(os.Stderr, nil))),
		kit.WithPlugins(
			health.New(health.Check("database", store.Ping)),
			requestlog.New(requestlog.SkipAssets()),
			compress.New(),
			secure.New(),
		),
	)
	if err != nil {
		log.Fatal(err)
	}
	if err := app.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
