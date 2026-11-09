# 0003. Route server files are Go packages wired up by code generation

- Status: Proposed
- Date: 2026-11-09

## Context

SvelteKit colocates server code with routes (`blog/[slug]/+page.server.ts`). We want the same with `+page.server.go`, but Go imposes constraints:

- Every directory is its own package, and the app's `main` must import each route package to call it.
- Folder names such as `[slug]`, `[...rest]` and `(group)` contain characters that are invalid or unreliable in Go import paths across tools.
- A route folder may contain both `+page.server.go` and `+layout.server.go`, which must not clash.

## Decision (proposed)

- Each route folder containing `.go` files is a Go package. Authors may name the package anything; `page` is the convention.
- `tern gen` writes `.tern/manifest.go`, which wires every route's `Load`, `Actions` and handlers into the router. Where a folder name is not a valid import path, the generator mirrors that package into `.tern/routes/` under a sanitized path (e.g. `blog/_slug_`) and imports the mirror.
- Layout and page functions in the same folder are distinguished by name: `Load`/`Actions` for the page, `LayoutLoad` for the layout.

The mirroring approach needs a prototype before this ADR is accepted: we must confirm it keeps compiler errors, `go vet` and editor navigation pointing at the user's original files (for example via `//line` directives).

## Consequences

- Authors keep SvelteKit-style folder names.
- Codegen becomes a required step; `tern dev` runs it automatically.
- If mirroring proves too fragile, the fallback is a sanitized naming convention for dynamic folders, which would be a visible difference from SvelteKit.
