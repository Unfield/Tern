// Package about is a static page, prerendered to HTML by `tern build`.
package about

import "github.com/Unfield/Tern/kit"

const Prerender = true

type About struct {
	Features []string `json:"features"`
}

func Load(ctx *kit.Ctx) (About, error) {
	return About{Features: []string{"Svelte on the client", "Go on the server", "One binary"}}, nil
}
