// Package stats serves GET /api/stats as JSON, a typed endpoint: tern gen
// describes it in the OpenAPI document and the $api client.
package stats

import (
	"github.com/Unfield/Tern/kit"

	"dashboard/internal/app"
	"dashboard/internal/db"
)

func GET(ctx *kit.Ctx) (db.Stats, error) {
	user, store, err := app.User(ctx)
	if err != nil {
		return db.Stats{}, err
	}
	return store.Stats(ctx, user)
}
