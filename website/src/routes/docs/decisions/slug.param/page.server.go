// Package decision is one of the decision records.
package decision

import (
	"errors"

	"github.com/Unfield/Tern/kit"

	"github.com/Unfield/Tern/website/internal/docs"
)

const Prerender = true

func Entries(ctx *kit.Ctx) ([]map[string]string, error) {
	slugs, err := docs.Slugs("decisions/")
	out := make([]map[string]string, len(slugs))
	for i, s := range slugs {
		out[i] = map[string]string{"slug": s}
	}
	return out, err
}

func Load(ctx *kit.Ctx) (*docs.Doc, error) {
	d, err := docs.Get("decisions/" + ctx.Param("slug"))
	if errors.Is(err, docs.ErrNotFound) {
		return nil, kit.NotFound()
	}
	return d, err
}
