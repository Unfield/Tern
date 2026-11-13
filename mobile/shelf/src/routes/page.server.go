package routes

import (
	"strings"

	"github.com/Unfield/Tern/kit"

	"shelf/internal/shelf"
)

// Access keeps the shelf, and every route below it, to signed-in users.
var Access = kit.SignedIn

type Data struct {
	Links []shelf.Link `json:"links"`
	Show  string       `json:"show"`
}

func Load(ctx *kit.Ctx) (Data, error) {
	// Signed out, Require returns a redirect to the sign-in page.
	user, err := shelf.Accounts.Require(ctx)
	if err != nil {
		return Data{}, err
	}
	show := ctx.URL().Query().Get("show")
	if show != "read" && show != "all" {
		show = "unread"
	}
	links, err := shelf.List(ctx, user.ID, show)
	if err != nil {
		return Data{}, err
	}
	return Data{Links: links, Show: show}, nil
}

// NewLink is the form that adds a link. Its tags are the rules: Tern
// checks them on the server, and in the browser as the user types.
type NewLink struct {
	URL   string `form:"url" validate:"required,url,max=2000" label:"Address"`
	Title string `form:"title" validate:"max=200"`
}

type Added struct {
	Link shelf.Link `json:"link"`
}

var Actions = kit.Actions{
	"add": kit.FormAction(func(ctx *kit.Ctx, in NewLink) (Added, error) {
		user, err := shelf.Accounts.Require(ctx)
		if err != nil {
			return Added{}, err
		}
		link, err := shelf.Add(ctx, user.ID, in.URL, strings.TrimSpace(in.Title))
		if err != nil {
			return Added{}, err
		}
		return Added{Link: link}, nil
	}),
}
