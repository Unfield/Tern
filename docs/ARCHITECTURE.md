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

Every page is **rendered on the server and hydrated** in the browser.

1. Go matches the route and runs the loads, producing a payload: the route ID, parameters, and each node's JSON data.
2. The payload goes to the server renderer, a Svelte bundle running in a pool of embedded QuickJS VMs ([ADR 0004](adr/0004-ssr-with-embedded-quickjs.md)). It renders the same component tree the client will, and returns `head` and `body` HTML.
3. Go writes `src/app.html` with the rendered HTML, the payload as `<script type="application/json">`, and the scripts and styles the route needs.
4. The client reads the payload and hydrates.

On client-side navigation, the router requests `/<path>/__data.json` and renders without a full reload.

The server and the client use one function to decide what renders for a payload (`runtime/select.ts`), so hydration always matches. If a server render fails or times out, the page is sent without HTML and renders on the client; the error is logged. A page can opt out with `const SSR = false` in its route package, and an app with `kit.WithoutSSR()`.

In development, the Go app asks Vite to render, so server rendering always uses the latest source.

**Streaming.** A load can return slow data as `kit.Deferred[T]`, created with `kit.Defer(ctx, fn)`. The value is a placeholder in the payload and a `Promise<T>` in Svelte. The page renders and is sent without waiting (the server renders `{#await}` blocks in their pending state), then each value is written as it resolves: as an inline `<script>` before `</body>` on full page loads, or as one JSON line per value after the payload in `__data.json`.

### 6. Client runtime (`packages/tern`, published as `@ternkit/tern`)

- **Router:** link interception, history, scroll restoration, `goto`, `invalidate`, `invalidateAll`, preloading on hover.
- **State:** `page` (url, params, data, form, status, error) and `navigating`, exposed as Svelte 5 reactive state.
- **Forms:** `use:enhance` submits actions with `fetch` and applies the result; without JavaScript, forms post normally and Go handles them.
- **Vite plugin (`@ternkit/tern/vite`):** reads the route tree, builds per-route entry chunks, and writes a manifest Go uses to emit the right script and CSS tags.

The client API mirrors SvelteKit's names and semantics where possible, so existing knowledge and docs carry over.

### 7. Dev server (`cmd/tern/dev.go`)

`tern dev` serves everything on **one port** (5173 by default):

```
browser ──▶ tern dev (5173) ──▶ Go app (internal port) ──▶ Vite (internal port)
             │                     handles pages, data,       modules, assets,
             │                     actions, endpoints          HMR websocket
             └─ /__tern/dev/*: reload events, the error page, the log view,
                and the dev tools overlay and its API
```

- The Go app is rebuilt and restarted when `.go`, `go.mod` or `go.sum` files change. Requests made during a restart wait for the new process instead of failing, and open pages reload once it is up.
- Adding or removing route files reruns codegen; the server restarts only if the generated Go code changed.
- Svelte, CSS and TypeScript changes go straight to Vite's hot module replacement.
- If code generation or the Go build fails, every page shows the error until it is fixed, then reloads by itself.
- Every page gets the dev tools overlay ([Dev tools](dev-tools.md)): payloads carry their request ID and route files under `tern dev`, the router and forms announce each page and action with a `tern:dev` event, and the overlay reads the request's logs, spans and error reports from `tern dev`.

### 8. Build (`internal/build`)

1. `tern gen`
2. `vite build` → `.tern/output/client`
3. Precompress text assets over 1 KiB with gzip
4. `go build -trimpath` with the client embedded via `embed.FS`

Output: one static binary serving the app, with hashed immutable assets and correct cache headers.

Apps pin the CLI in their `go.mod` with `go get -tool github.com/Unfield/Tern/cmd/tern`, so `go tool tern` always matches the version of `kit` they import.

## Project layout of a Tern app

```
my-app/
├── src/
│   ├── routes/          # pages, layouts, server files
│   ├── lib/             # shared Svelte/TS code ($lib)
│   ├── app.html         # HTML template
├── static/              # copied as-is
├── .tern/               # generated; gitignored
├── main.go              # calls ternApp() and ListenAndServe
├── tern.gen.go          # generated; gitignored
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
