# 0023. Apps over MCP

- Status: Accepted
- Date: 2026-11-12

## Context

Assistants such as Claude and ChatGPT use apps through the Model Context Protocol: an app offers tools, and the assistant calls them for the user. For a Tern app, that means letting an assistant save a link, look up an invoice or file an issue, as the signed-in user and with only that user's permissions.

`tern mcp` (the CLI) already serves MCP for coding tools, over stdio on the developer's machine. Serving a deployed app is a different problem:

- **The transport is HTTP**, and anyone on the internet can reach it.
- **Users authorize assistants with OAuth.** The MCP server is an OAuth resource server, and the authorization server issues tokens for it.
- **The protocol is in transition.** The 2026-07-28 revision is stateless: there's no `initialize` handshake, every request carries its version in `_meta` and in headers, and servers must implement `server/discover`. Clients still in use speak 2025-11-25 and earlier, which start with `initialize`. A server may serve both on one endpoint.
- **Client registration changed.** Assistants can't be listed in an app's configuration ahead of time. The 2026-07-28 revision prefers Client ID Metadata Documents: the client's ID is an HTTPS URL, and the authorization server fetches the client's metadata from it. Dynamic Client Registration (RFC 7591) is deprecated.

Tern already has most of the authorization side: `plugins/oidc` is an OAuth 2.1 and OpenID Connect provider with PKCE, refresh token rotation, consent and JWT access tokens.

## Decision

- **A `mcp` package serves an app's tools over Streamable HTTP**, as a plugin at `/mcp`. A tool is a Go function with typed input and output, registered with `mcp.Tool(name, description, fn)`:
  - **Schemas from Go types.** The input and output schemas come from the structs by reflection, with `json` names, `description` tags, and the `validate` rules forms already use, which are also checked before the function runs.
  - **The same code as pages.** A tool gets a `*kit.Ctx` with the user signed in, so it calls the same store functions and permission checks as the app's actions. The app's hooks and plugins run first, as for any request.
  - **Tools are listed explicitly.** Actions and endpoints aren't exposed automatically: what an assistant may do is a decision, and a tool's description is written for a model, not a form.
- **Both protocol eras on one endpoint:** 2026-07-28 (stateless, `server/discover`, request headers checked against the body), and 2025-11-25, 2025-06-18 and 2025-03-26 (with `initialize`), all without sessions. Responses are JSON; no SSE streams, since tools return one result.
- **Authorization through `plugins/oidc`:**
  - The MCP plugin serves Protected Resource Metadata (RFC 9728) and answers 401 with the `WWW-Authenticate` challenge clients follow.
  - The provider accepts resource indicators (RFC 8707). Tokens for a resource carry it as their audience, and the MCP server rejects tokens issued for anything else.
  - The provider supports Client ID Metadata Documents, opt-in, so assistants register themselves. It fetches the documents with protection against server-side request forgery: HTTPS only, public addresses only, no redirects, a size limit and a timeout.
  - Clients from metadata documents are always public, use PKCE, and get a consent screen that names them and the site they return to.
  - It serves RFC 8414 authorization server metadata beside OpenID discovery.
  - Dynamic Client Registration is not implemented, since the protocol deprecates it. Clients that need it can be listed in `Options.Clients`.
- **Without the provider,** `Options.Authorize` lets an app check requests its own way, such as with API keys.

## Consequences

- An app becomes usable from assistants with a page of Go, and users approve each assistant on the app's own consent screen.
- Tools are another way in, so they need the same checks as actions. A tool that skips `Require` is open to anyone with a token; the docs show the check in every example.
- Fetching client metadata makes the provider fetch URLs that strangers name. The protections against server-side request forgery are what make that safe, and the feature is off unless enabled.
- When the 2025 protocol versions fall out of use, their `initialize` path can be removed without changing tools.
