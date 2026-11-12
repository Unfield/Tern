// Package step is one of the tutorial's steps.
package step

import (
	"errors"

	"github.com/Unfield/Tern/kit"

	"github.com/Unfield/Tern/website/internal/docs"
)

const Prerender = true

func Entries(ctx *kit.Ctx) ([]map[string]string, error) {
	slugs, err := docs.Slugs("tutorial/")
	out := make([]map[string]string, len(slugs))
	for i, s := range slugs {
		out[i] = map[string]string{"slug": s}
	}
	return out, err
}

func Load(ctx *kit.Ctx) (*docs.Doc, error) {
	d, err := docs.Get("tutorial/" + ctx.Param("slug"))
	if errors.Is(err, docs.ErrNotFound) {
		return nil, kit.NotFound()
	}
	return d, err
}
