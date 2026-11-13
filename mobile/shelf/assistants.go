package shelfapp

import (
	"errors"
	"os"
	"strconv"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/mcp"
	"github.com/Unfield/Tern/plugins/oidc"

	"shelf/internal/shelf"
)

// Assistants such as Claude use the shelf through MCP, at /mcp. Users
// approve each assistant on the app's consent screen, and its tools run as
// that user, with the same checks as the pages. See docs/mcp-server.md.

// assistants returns the OAuth provider assistants sign users in with, and
// the MCP server they call.
func assistants() (*oidc.Plugin, *mcp.Server, error) {
	store, err := oidc.NewSQLStore(shelf.DB)
	if err != nil {
		return nil, nil, err
	}
	// The app's public address: tokens name it, and clients check it.
	issuer := os.Getenv("BASE_URL")
	if issuer == "" {
		issuer = "http://localhost:5173"
	}
	provider := oidc.New(oidc.Options{
		Issuer:   issuer,
		Store:    store,
		LoginURL: "/login",
		// Assistants register themselves with a metadata document at
		// their client ID's URL, and users approve each one.
		ClientMetadataDocuments: true,
		Authenticate: func(ctx *kit.Ctx) (string, error) {
			if u := accounts.Current(ctx); u != nil {
				return strconv.FormatInt(u.ID, 10), nil
			}
			return "", nil
		},
	})
	server := mcp.New(mcp.Options{
		Name:         "shelf",
		Title:        "Shelf",
		Instructions: "Shelf is the user's reading list: links they saved to read later. Save a link when the user wants to read something later, and list the unread ones when they ask what to read.",
		Tools:        tools,
		OAuth:        provider,
		// The token's subject is the user's ID: make them the request's
		// signed-in user, so tools use accounts as pages do.
		SignIn: func(ctx *kit.Ctx, t *oidc.AccessToken) error {
			id, err := strconv.ParseInt(t.Subject, 10, 64)
			if err != nil {
				return err
			}
			u, err := shelf.Accounts.UserByID(ctx, id)
			if err != nil {
				return err
			}
			if u == nil {
				return errors.New("the user no longer exists")
			}
			shelf.Accounts.ActAs(ctx, u)
			return nil
		},
	})
	return provider, server, nil
}

type ListArgs struct {
	Show string `json:"show,omitempty" validate:"oneof=unread|read|all" description:"Which links: unread (the default), read or all"`
}

type Links struct {
	Links []shelf.Link `json:"links"`
}

type SaveArgs struct {
	URL   string `json:"url" validate:"required,url,max=2000" description:"The page's address"`
	Title string `json:"title,omitempty" validate:"max=200" description:"A title; the address if left out"`
}

type ReadArgs struct {
	ID   int64 `json:"id" validate:"required" description:"The link's id, from list_links"`
	Read bool  `json:"read" description:"true to mark it read, false for unread"`
}

type Done struct {
	OK bool `json:"ok"`
}

var tools = []mcp.Tool{
	mcp.NewTool("list_links", "List the links on the user's shelf, newest first.",
		func(ctx *kit.Ctx, in ListArgs) (Links, error) {
			user, err := shelf.Accounts.Require(ctx)
			if err != nil {
				return Links{}, err
			}
			show := in.Show
			if show == "" {
				show = "unread"
			}
			links, err := shelf.List(ctx, user.ID, show)
			return Links{Links: links}, err
		}, mcp.Title("List links"), mcp.ReadOnly()),

	mcp.NewTool("save_link", "Save a link to the user's shelf, to read later.",
		func(ctx *kit.Ctx, in SaveArgs) (shelf.Link, error) {
			user, err := shelf.Accounts.Require(ctx)
			if err != nil {
				return shelf.Link{}, err
			}
			return shelf.Add(ctx, user.ID, in.URL, in.Title)
		}, mcp.Title("Save a link"), mcp.Additive()),

	mcp.NewTool("mark_read", "Mark one of the user's links read, or unread.",
		func(ctx *kit.Ctx, in ReadArgs) (Done, error) {
			user, err := shelf.Accounts.Require(ctx)
			if err != nil {
				return Done{}, err
			}
			err = shelf.SetRead(ctx, user.ID, in.ID, in.Read)
			if errors.Is(err, shelf.ErrNotFound) {
				return Done{}, kit.Error(404, "There's no link with that id.")
			}
			return Done{OK: err == nil}, err
		}, mcp.Title("Mark read"), mcp.Idempotent(), mcp.Additive()),
}
