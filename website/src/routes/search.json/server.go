// Package search serves the search index, prerendered to /search.json.
package search

import (
	"net/http"

	"github.com/Unfield/Tern/kit"

	"github.com/Unfield/Tern/website/internal/docs"
)

const Prerender = true

func GET(ctx *kit.Ctx) error {
	idx, err := docs.Index()
	if err != nil {
		return err
	}
	ctx.SetHeader("Cache-Control", "public, max-age=3600")
	return ctx.JSON(http.StatusOK, idx)
}
