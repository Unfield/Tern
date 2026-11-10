# The MCP server

`tern mcp` serves the [Model Context Protocol](https://modelcontextprotocol.io) on stdin and stdout, so AI coding tools can work with a Tern app: list its routes and their types, scaffold routes and live components, run code generation, type checks, tests and builds, and read Tern's documentation for the version the app uses.

## Setup

Run it from the app's root, with the CLI version pinned in `go.mod`:

- **Projects from `tern new`:** they include a `.mcp.json`, which Claude Code and other tools that read it pick up.
- **Claude Code:** `claude mcp add tern -- go tool tern mcp`
- **Other tools:** add a stdio server with the command `go` and the arguments `tool tern mcp`. For example, in a tool that uses `mcpServers`:

  ```json
  {
  	"mcpServers": {
  		"tern": { "command": "go", "args": ["tool", "tern", "mcp"] }
  	}
  }
  ```

## Tools

| Tool | What it does |
|---|---|
| `project` | the Go module, Tern version, routes, live components, `tern.json`, package scripts and the Tern packages the app imports |
| `routes` | every route: URL pattern, directory, kind, files, `Load`, `Actions`, `LayoutLoad`, endpoint methods, layouts |
| `route_types` | the generated `$types.d.ts` for a route, by pattern (`/blog/[slug]`) or ID |
| `live_components` | components with `<script lang="go">`, with the TypeScript types of their state and Funcs |
| `generate` | `tern gen`, reporting changed files and warnings |
| `check` | `tern check`, returning the errors with file and line |
| `go_vet`, `go_test` | `go vet` and `go test` on a package pattern, optionally `-run` |
| `build` | `tern build` |
| `new_route` | creates a route from a SvelteKit-style path, such as `/blog/[slug]` or `/(app)/settings`, with a page, a `Load`, a form action, endpoint methods or a live page; existing files are kept |
| `new_live_component` | creates a `.svelte` file with `<script lang="go" live>` to start from |
| `new_migration` | `tern migrate new` |
| `search_docs`, `read_doc` | the guide, live components, deployment, the SvelteKit migration guide and the decision records, embedded in the CLI |

The docs are also resources, at `tern://docs/guide.md` and so on. The server's instructions tell the model Tern's conventions, such as the Go-safe directory names.

## Safety

- **Tool hints:** tools that change files or run builds are not marked read-only, so clients that ask before running such tools will ask.
- **Paths:** the scaffolding tools write only under `src/` and `migrations/`, and never overwrite a file.
- **No network:** the server makes no network requests of its own.
