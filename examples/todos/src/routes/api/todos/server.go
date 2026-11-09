// Package todos is a JSON endpoint: GET /api/todos.
package todos

import (
	"net/http"

	"github.com/Unfield/Tern/kit"

	"todos/internal/session"
	"todos/internal/store"
)

func GET(ctx *kit.Ctx) error {
	user := session.User(ctx)
	if user == "" {
		return kit.Error(http.StatusUnauthorized, "Sign in first")
	}
	return ctx.JSON(http.StatusOK, store.Default.List(user))
}
