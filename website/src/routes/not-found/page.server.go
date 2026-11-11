// Package notfound is the page static hosts show for a missing URL: the
// Pages workflow copies it to 404.html.
package notfound

import "github.com/Unfield/Tern/kit"

const Prerender = true

type NotFound struct{}

func Load(ctx *kit.Ctx) (NotFound, error) { return NotFound{}, nil }
