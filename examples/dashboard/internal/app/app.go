// Package app holds helpers shared by the dashboard's routes.
package app

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Unfield/Tern/kit"

	"dashboard/internal/db"
	"dashboard/internal/session"
)

// User returns the signed-in user and the database, or redirects to the
// login page.
func User(ctx *kit.Ctx) (string, *db.Store, error) {
	user := session.User(ctx)
	if user == "" {
		return "", nil, kit.Redirect(http.StatusSeeOther, "/login")
	}
	store, err := db.Default()
	return user, store, err
}

// NotFound turns db.ErrNotFound into a 404.
func NotFound(err error) error {
	if errors.Is(err, db.ErrNotFound) {
		return kit.NotFound()
	}
	return err
}

// FormID reads a positive integer form field.
func FormID(ctx *kit.Ctx, field string) (int64, error) {
	form, err := ctx.Form()
	if err != nil {
		return 0, err
	}
	id, err := strconv.ParseInt(form.Get(field), 10, 64)
	if err != nil || id <= 0 {
		return 0, kit.Error(http.StatusBadRequest, "Invalid "+field)
	}
	return id, nil
}
