// Package routes sends visitors to where they belong: setup on a new
// install, sign-in, or the sites.
package routes

import (
	"net/http"

	"github.com/Unfield/Tern/kit"

	"analytics/internal/app"
)

func Load(ctx *kit.Ctx) (struct{}, error) {
	if _, ok := app.CurrentUser(ctx); ok {
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/sites")
	}
	n, err := app.Store.CountUsers(ctx)
	if err != nil {
		return struct{}{}, err
	}
	if n == 0 {
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/setup")
	}
	return struct{}{}, kit.Redirect(http.StatusSeeOther, "/login")
}
