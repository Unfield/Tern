// Package docs is the documentation's layout: the sidebar.
package docs

import (
	"github.com/Unfield/Tern/kit"

	"github.com/Unfield/Tern/website/internal/docs"
)

type Layout struct {
	Nav []docs.Section `json:"nav"`
}

func LayoutLoad(ctx *kit.Ctx) (Layout, error) {
	nav, err := docs.Nav()
	return Layout{Nav: nav}, err
}
