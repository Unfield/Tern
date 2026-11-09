# Architecture

This document describes how Tern is put together. It covers the target design for **v0.1** and notes where v0.2 (server-side rendering) plugs in. Decisions with real tradeoffs are recorded separately in [`docs/adr`](adr).

## Goals

1. **SvelteKit's developer experience, Go's runtime.** Route files, `load`, actions and layouts should feel familiar to SvelteKit users.
2. **Real Svelte.** Tern uses the official Svelte 5 compiler and Vite. It never forks or reimplements Svelte.
3. **Types across the boundary.** Data returned from Go is typed in Svelte without hand-written interfaces.
4. **Plain Go.** The server is `net/http`. A Tern app is an `http.Handler` and can be mounted anywhere.
5. **One artifact.** Production builds are a single binary with the client embedded.

## Non-goals (for now)

- Running server logic in JavaScript (no `+page.server.ts`; `Load` is always Go).
- Supporting component frameworks other than Svelte.
- Edge or serverless runtimes before the core is stable.

## The big picture

```
                ┌──────────────── build time ────────────────┐
 src/routes/**  │  tern gen                                   │
 ──────────────▶│   ├─ scan routes        → route tree        │
                │   ├─ analyze Go (types) → $types.d.ts       │
                │   └─ emit manifest      → .tern/manifest.go │
                │  vite build             → .tern/client/**   │
                │  go build (embed)       → single binary     │
                └─────────────────────────────────────────────┘

                ┌──────────────── run time ──────────────────┐
  request ─────▶│  hooks/middleware                           │
                │   └─ router match                           │
                │       ├─ action (POST) ─┐                   │
                │       └─ layout + page Load (in parallel)   │
                │            │                                │
                │   ┌────────┴─────────┐                      │
                │   │ document request │ data request         │
                │   │  HTML shell +    │ __data.json          │
                │   │  inlined JSON    │                      │
                │   └──────────────────┘                      │
                └─────────────────────────────────────────────┘
                           │
                     browser: Svelte app + Tern client router
```

## Components

### 1. CLI (`cmd/tern`)

| Command | Purpose |
|---|---|
| `tern new <dir>` | Scaffold a project from a template |
| `tern dev` | Run the dev server with hot reload |
| `tern gen` | Regenerate the manifest and TypeScript types |
| `tern build` | Produce a production binary |

The CLI is a thin layer over packages in `internal/`; all logic is testable without it.

### 2. Routing (`internal/routes`)

Routes come from the filesystem. Each directory under `src/routes` is a URL segment:

| File | Role |
|---|---|
| `+page.svelte` | Page component |
| `+layout.svelte` | Layout wrapping this directory and its children |
| `+error.svelte` | Error boundary |
| `*.go` | The route's Go package (conventionally `page.server.go`, `layout.server.go`, `server.go`) |

The Go package's role comes from what it exports: `Load` and `Actions` for the page, `LayoutLoad` for the layout, and `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `OPTIONS`, `HEAD` for an endpoint.

Dynamic segments use suffixes, because Go import paths cannot contain brackets ([ADR 0003](adr/0003-route-files-as-go-packages.md)):

| Directory | Pattern | Matches |
|---|---|---|
| `about` | `/about` | `/about` |
| `slug.param` | `/[slug]` | `/hello` |
| `id.param.integer` | `/[id=integer]` | `/42`, if the `integer` matcher accepts it |
| `lang.optional` | `/[[lang]]` | `/` and `/en` |
| `path.rest` | `/[...path]` | `/`, `/a`, `/a/b/c` |
| `app.group` | (none) | groups routes, e.g. to share a layout |

Matching priority follows SvelteKit: static segments beat parameters, parameters with a matcher beat those without, required beats optional, and rest parameters come last.

The scanner produces a **route tree** that is the single source of truth for codegen, the dev server and the client router.

### 3. Server runtime (`kit`)

`kit` is the only Go package app authors import. Its surface is intentionally small:

```go
// Load runs on every document and data request for the route.
func Load(ctx *kit.Ctx) (Data, error)

// Actions handle form POSTs. "default" handles a form with no ?/name.
var Actions = kit.Actions{
	"default": func(ctx *kit.Ctx) kit.ActionResult { ... },
}
```

- `kit.Ctx` wraps the request and response: params, URL, cookies, headers, `Locals`, and `Parent()` to read layout data.
- Control flow uses returned errors: `kit.Redirect(303, "/login")`, `kit.NotFound()`, `kit.Error(status, msg)`.
- Action failures return `kit.Fail(400, data)` so the page can re-render with validation errors.
- Layout and page `Load` functions run **concurrently** where they do not call `Parent()`.

**Hooks.** An app can provide `Handle(ctx, next)` for request-wide logic such as auth or logging. Because a Tern app is an `http.Handler`, standard `func(http.Handler) http.Handler` middleware works as well.

### 4. Type generation (`internal/typegen`)

`tern gen` loads the route packages with `golang.org/x/tools/go/packages`, finds each `Load`'s return type, and writes a `$types.d.ts` next to the route (mirrored into `.tern/types`, as SvelteKit does):

| Go | TypeScript |
|---|---|
| `string`, `bool` | `string`, `boolean` |
| integer and float kinds | `number` |
| `time.Time` | `string` (ISO 8601) |
| `[]T`, `map[string]T` | `T[]`, `Record<string, T>` |
| `*T` | `T \| null` |
| struct fields | follows `json` tags, including `omitempty` → optional |
| `json.Marshaler` types | `unknown` unless annotated |

Layout data is merged into child page types, so `data` in a page includes everything its layouts load.

### 5. Rendering

**v0.1: client-side rendering.** On a document request, Go runs the matching `Load` functions and responds with an HTML shell that contains:

- the Vite-built entry script and CSS links for the route
- the merged load data as `<script type="application/json">`
- a `<div>` mount point

The Tern client reads the data and mounts the layout and page components. On client-side navigation, the router requests `/<path>/__data.json` and swaps components without a full reload.

**v0.2: server-side rendering.** The same request pipeline runs Svelte's server-compiled components inside an embedded JavaScript engine to produce HTML, and the client hydrates instead of mounting. Because `Load` only returns data, **no app code changes between v0.1 and v0.2**. See [ADR 0002](adr/0002-client-rendering-first.md).

### 6. Client runtime (`packages/tern`, published as `@unfield/tern`)

- **Router:** link interception, history, scroll restoration, `goto`, `invalidate`, `invalidateAll`, preloading on hover.
- **State:** `page` (url, params, data, form, status, error) and `navigating`, exposed as Svelte 5 reactive state.
- **Forms:** `use:enhance` submits actions with `fetch` and applies the result; without JavaScript, forms post normally and Go handles them.
- **Vite plugin (`@unfield/tern/vite`):** reads the route tree, builds per-route entry chunks, and writes a manifest Go uses to emit the right script and CSS tags.

The client API mirrors SvelteKit's names and semantics where possible, so existing knowledge and docs carry over.

### 7. Dev server (`internal/dev`)

`tern dev` serves everything on **one port**:

- The Go app runs as a child process and is rebuilt and restarted when `.go` files change.
- Requests for modules, assets and HMR are proxied to a Vite dev server.
- Adding or removing route files reruns codegen automatically.
- Go compile errors are shown in the browser as an overlay.

### 8. Build (`internal/build`)

1. `tern gen`
2. `vite build` → `.tern/client`
3. `go build` with the client embedded via `embed.FS`

Output: one static binary serving the app, with hashed immutable assets and correct cache headers.

## Project layout of a Tern app

```
my-app/
├── src/
│   ├── routes/          # pages, layouts, server files
│   ├── lib/             # shared Svelte/TS code ($lib)
│   ├── app.html         # HTML template
│   └── hooks.server.go  # optional Handle hook
├── static/              # copied as-is
├── .tern/               # generated; gitignored
├── main.go              # tiny: tern.Serve(app)
├── go.mod
├── package.json
└── vite.config.ts
```

## Repository layout

```
cmd/tern/            CLI
kit/                 public Go runtime
internal/routes/     route scanning and matching
internal/typegen/    Go → TypeScript
internal/codegen/    manifest generation
internal/dev/        dev server
internal/build/      production builds
packages/tern/       client runtime + Vite plugin (npm)
examples/            example apps (also used in end-to-end tests)
```

## Testing strategy

- **Unit tests** for the route scanner, matcher and typegen, with golden files for generated output.
- **Integration tests** that run the request pipeline against `httptest` servers.
- **End-to-end tests** that build the `examples/` apps and drive them with Playwright, covering both JavaScript-enabled and JavaScript-disabled flows.
