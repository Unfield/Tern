// The docs' first page, from the README.
package docs

import (
	"github.com/Unfield/Tern/kit"

	"github.com/Unfield/Tern/website/internal/docs"
)

const Prerender = true

func Load(ctx *kit.Ctx) (*docs.Doc, error) {
	return docs.Get("")
}
