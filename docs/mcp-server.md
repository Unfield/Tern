# Apps for assistants (MCP)

The `mcp` package lets AI assistants, such as Claude and ChatGPT, use your app for its users: save a link, look up an order, file an issue. It serves the app's **tools** over the [Model Context Protocol](https://modelcontextprotocol.io), at `/mcp`. A user adds the app to their assistant and approves it on the app's own consent screen. From then on, the assistant calls the tools as that user, with that user's permissions.

This page is about serving MCP from a deployed app. For MCP in your editor while you build an app, see [AI coding tools](mcp.md).

## Tools

A tool is a Go function with typed arguments and a typed result, like a typed endpoint:

```go
type SaveArgs struct {
	URL   string `json:"url" validate:"required,url,max=2000" description:"The page's address"`
	Title string `json:"title,omitempty" validate:"max=200" description:"A title; the address if left out"`
}

var saveLink = mcp.NewTool("save_link", "Save a link to the user's shelf, to read later.",
	func(ctx *kit.Ctx, in SaveArgs) (shelf.Link, error) {
		user, err := shelf.Accounts.Require(ctx)
		if err != nil {
			return shelf.Link{}, err
		}
		return shelf.Add(ctx, user.ID, in.URL, in.Title)
	}, mcp.Title("Save a link"), mcp.Additive())
```

- **Schemas from Go.** The arguments struct becomes the tool's input schema: `json` names, `description` tags, and the [`validate` rules](guide.md#validating-forms) forms use, as lengths, ranges, formats and choices. A struct result also gets an output schema.
- **Checked before your code runs.** Arguments are decoded strictly, so an unknown field is an error. The `validate` rules and a `Validate()` method are checked as for a typed endpoint. Failures go back to the assistant as tool errors, so it can correct the call.
- **The user is signed in.** `ctx` is the request's, with the token's user signed in, so tools call the same code and permission checks as your pages. Check the user in every tool, as you would in every action: a tool is another way in.
- **Errors:** messages from `kit.Error` and `kit.NotFound` reach the assistant. Other errors are logged, and the assistant sees "Internal Error". A redirect, such as `Require` with nobody signed in, becomes "This needs a signed-in user."
- **Hints:** `mcp.ReadOnly()` for tools that change nothing, `mcp.Additive()` for tools that only add, `mcp.Idempotent()`, and `mcp.Destructive()`. Assistants use them to decide when to ask the user first. Tools that aren't read-only count as destructive unless marked additive.
- **Names:** letters, digits, `_`, `-` and `.`, as in `save_link`. The description is written for a model: say what the tool does and when to use it.

## The server

```go
server := mcp.New(mcp.Options{
	Name:         "shelf",
	Title:        "Shelf",
	Instructions: "Shelf is the user's reading list. Save a link when they want to read something later.",
	Tools:        []mcp.Tool{listLinks, saveLink, markRead},
	OAuth:        provider,
	SignIn:       signIn, // below
})
app, err := ternApp(kit.WithPlugins(acc, provider, server))
```

`Instructions` tell the assistant's model what the app is and how its tools fit together.

## Signing users in

Assistants authorize with OAuth, and your app is the authorization server, through [`plugins/oidc`](guide.md#be-an-openid-connect-provider). With [accounts](accounts.md):

```go
store, err := oidc.NewSQLStore(db)
provider := oidc.New(oidc.Options{
	Issuer:   "https://shelf.example.com", // the app's public address
	Store:    store,
	LoginURL: "/login",
	// Assistants register themselves; users approve each one.
	ClientMetadataDocuments: true,
	Authenticate: func(ctx *kit.Ctx) (string, error) {
		if u := accounts.Current(ctx); u != nil {
			return strconv.FormatInt(u.ID, 10), nil
		}
		return "", nil
	},
})

// signIn makes the token's user the request's.
func signIn(ctx *kit.Ctx, t *oidc.AccessToken) error {
	id, err := strconv.ParseInt(t.Subject, 10, 64)
	if err != nil {
		return err
	}
	u, err := acc.UserByID(ctx, id)
	if err != nil || u == nil {
		return errors.New("no such user")
	}
	acc.ActAs(ctx, u)
	return nil
}
```

What happens when a user adds the app to an assistant:

1. The assistant calls `/mcp` without a token and gets 401, with a `WWW-Authenticate` header pointing at `/.well-known/oauth-protected-resource/mcp`. That document names your app as the authorization server.
2. It reads your app's metadata at `/.well-known/oauth-authorization-server`, and sees it can use its own **client ID metadata document**: its client ID is an HTTPS URL, such as `https://claude.ai/oauth/mcp-client-metadata`, where it publishes its name and redirect addresses. Your app fetches that document; nothing has to be registered ahead of time.
3. It sends the user to `/oauth/authorize`. The user signs in if they need to, and sees your consent screen. It names the assistant and the site they'll return to, because anyone can publish a metadata document with any name.
4. The assistant gets a token bound to your MCP server's address (a resource indicator, RFC 8707) and calls the tools with it. A token issued for anything else, even by the same provider, is refused.

**Fetching metadata documents safely.** With `ClientMetadataDocuments`, your app fetches URLs that strangers name. It fetches only HTTPS on port 443, only from public addresses (checked on the address it actually connects to, so DNS can't point it at your network), without following redirects, up to 64 KB and within 5 seconds. It caches documents by their `Cache-Control`, between 5 minutes and a day. Clients described this way are always public, must use PKCE, may only redirect to HTTPS or to `localhost`, and always get the consent screen.

**Dynamic client registration** (RFC 7591) is deprecated by the protocol and not supported. To add a client that needs it, list it in `oidc.Options.Clients`.

## Scopes

`Options.Scopes` are scopes every request needs, and `mcp.Scopes("links:delete")` on a tool adds its own. A token without a tool's scopes gets 403 with `error="insufficient_scope"` and the scopes to ask for. The assistant then asks the user to approve them, and retries. Scopes must also be in `oidc.Options.Scopes`.

## Without OAuth

`Options.Authorize` checks requests your own way, such as an API key for scripts:

```go
Authorize: func(ctx *kit.Ctx) error {
	key, err := apiKeys.Find(ctx, ctx.Request.Header.Get("X-API-Key"))
	if err != nil {
		return kit.Error(401, "an API key is required")
	}
	acc.ActAs(ctx, key.Owner)
	return nil
},
```

With neither `OAuth` nor `Authorize`, anyone can call the tools, which suits only tools over public data.

## Protocol details

- **Versions.** The server speaks MCP 2026-07-28, which is stateless: every request carries its version, and `server/discover` describes the server. It also speaks 2025-11-25, 2025-06-18 and 2025-03-26, which start with `initialize`. Both work on the same endpoint, without sessions.
- **Transport.** Each request is a `POST` answered with JSON. There are no server-sent event streams, since tools return one result, and `GET` answers 405. 2026-07-28 requests must carry `MCP-Protocol-Version`, `Mcp-Method` and, for tool calls, `Mcp-Name`, matching the body, or they get 400.
- **Browsers.** Requests from a browser page on another origin are refused unless it's in `Options.AllowedOrigins`. That stops other sites from reaching a server through a visitor's browser.
- **Limits.** Requests are limited to 1 MB, and tools run under the app's usual request timeout.

The finished [examples/shelf](../examples/shelf) has three tools: listing, saving and marking links read. See `assistants.go` there. [ADR 0023](adr/0023-apps-over-mcp.md) explains the design.
