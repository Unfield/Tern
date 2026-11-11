// Package page is a page of the docs.
package page

import (
	"errors"
	"strings"

	"github.com/Unfield/Tern/kit"

	"github.com/Unfield/Tern/website/internal/docs"
)

const Prerender = true

// Entries lists the pages directly under /docs, so tern build prerenders
// them all.
func Entries(ctx *kit.Ctx) ([]map[string]string, error) {
	slugs, err := docs.Slugs("")
	var out []map[string]string
	for _, s := range slugs {
		if !strings.Contains(s, "/") {
			out = append(out, map[string]string{"slug": s})
		}
	}
	return out, err
}

func Load(ctx *kit.Ctx) (*docs.Doc, error) {
	d, err := docs.Get(ctx.Param("slug"))
	if errors.Is(err, docs.ErrNotFound) {
		return nil, kit.NotFound()
	}
	return d, err
}
