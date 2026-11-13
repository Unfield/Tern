package search

import (
	"net/http"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"shelf/internal/shelf"
)

type Query struct {
	Q string `json:"q"`
}

// GET finds the signed-in user's links as they type, for the search box.
func GET(ctx *kit.Ctx, in Query) ([]shelf.Result, error) {
	user := accounts.Current(ctx)
	if user == nil {
		return nil, kit.Error(http.StatusUnauthorized, "Sign in to search your shelf.")
	}
	results, _, err := shelf.Results(ctx, user.ID, in.Q, 8)
	return results, err
}
