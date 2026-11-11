package main

import (
	"log"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/logging"
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
		kit.WithLogger(logging.New(logging.Options{})),
		kit.WithOpenAPI("/api/openapi.json", "Acme Dashboard API", "1.0.0"),
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
