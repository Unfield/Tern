# 0003. Route directories are Go packages, with Go-safe names

- Status: Accepted
- Date: 2026-11-09

## Context

SvelteKit colocates server code with routes (`blog/[slug]/+page.server.ts`). We want the same with Go, so a route's `Load` sits next to its `+page.svelte`.

We tested what Go accepts:

| Name | Result |
|---|---|
| directories `[slug]`, `(app)` | `malformed import path`. Worse, `go build ./...`, `go vet ./...` and `go test ./...` fail for the **whole module**, and gopls cannot load the package. |
| directories `~slug`, `+slug`, `-slug` | `invalid input directory name` |
| directory `_slug` | builds, but `./...` patterns silently skip it, so its tests and vet never run |
| directories `slug.param`, `slug-x`, `slug_x` | work everywhere |
| file `+page.server.go` | `invalid input file name` |

Mirroring route packages into a generated tree with `//line` directives would fix compiler errors but not `go test ./...` or editor support for the original files, because the original directories would still be malformed packages.

## Decision

1. **Directory names use suffixes**, which are valid Go import path elements:

   | SvelteKit | Tern |
   |---|---|
   | `[slug]` | `slug.param` |
   | `[id=integer]` | `id.param.integer` |
   | `[[lang]]` | `lang.optional` |
   | `[...path]` | `path.rest` |
   | `(app)` | `app.group` |

   Segments that mix text and parameters (`[slug].json`) are not supported; an endpoint can do the same job. The scanner rejects SvelteKit names with an error that gives the Tern name.

2. **A route directory's Go code is an ordinary Go package.** File names are free; `page.server.go`, `layout.server.go` and `server.go` are the convention. Roles come from exported names:

   | Export | Role |
   |---|---|
   | `func Load` | page data |
   | `var Actions` | page form actions |
   | `func LayoutLoad` | layout data |
   | `func GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS`, `HEAD` | endpoint |

3. **Wiring is generated.** `tern gen` writes `tern.gen.go` into the app's root package. It imports each route package directly and registers its functions with the runtime.

## Consequences

- `go build ./...`, `go test ./...`, `go vet ./...`, gopls and golangci-lint all work on route code with no special setup.
- Compiler errors point at the user's own files.
- Page and layout loads in one directory share a package, which is why the layout's function is named `LayoutLoad`.
- Route URLs look different on disk from SvelteKit's. Patterns are still displayed in bracket notation (`/blog/[slug]`) wherever Tern prints them.
