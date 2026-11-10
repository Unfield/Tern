// Package project serves one project by ID.
package project

import (
	"github.com/Unfield/Tern/kit"

	"dashboard/internal/app"
	"dashboard/internal/db"
)

// Path holds the route parameter.
type Path struct {
	ID int64 `path:"id"`
}

func GET(ctx *kit.Ctx, in Path) (db.Project, error) {
	user, store, err := app.User(ctx)
	if err != nil {
		return db.Project{}, err
	}
	p, err := store.Project(ctx, user, in.ID)
	return p, app.NotFound(err)
}
