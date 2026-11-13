package search

import (
	"strings"

	"github.com/Unfield/Tern/kit"

	"shelf/internal/shelf"
)

type Data struct {
	Q       string         `json:"q"`
	Results []shelf.Result `json:"results"`
	Total   int            `json:"total"`
}

// Load is the results page, which the search box goes to on Enter, and
// the way to search without JavaScript.
func Load(ctx *kit.Ctx) (Data, error) {
	user, err := shelf.Accounts.Require(ctx)
	if err != nil {
		return Data{}, err
	}
	d := Data{Q: strings.TrimSpace(ctx.URL().Query().Get("q")), Results: []shelf.Result{}}
	if d.Q == "" {
		return d, nil
	}
	d.Results, d.Total, err = shelf.Results(ctx, user.ID, d.Q, 50)
	return d, err
}
