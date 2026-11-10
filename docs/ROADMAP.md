# Roadmap

This is the planned order of work. It changes as we learn; the [issue tracker](https://github.com/Unfield/Tern/issues) is the live view.

## v0.1: Foundations (client-rendered)

The goal: you can build a real app with Tern and deploy it as one binary.

- [x] Route scanner and matcher (`slug.param`, `lang.optional`, `path.rest`, `app.group`, `id.param.matcher`)
- [x] Manifest code generation for route packages
- [x] `kit`: `Ctx`, `Load`, layouts with `Parent()`, `Redirect`/`Error`/`NotFound`
- [x] Form actions with `Fail` and progressive enhancement
- [x] Endpoints (`GET`, `POST`, … in a route's Go package)
- [x] Hooks (`kit.WithHook`) and `net/http` middleware
- [x] Go → TypeScript type generation, including layout data merging
- [x] Client runtime: router, `goto`, `invalidateAll`, `page` and `navigating` state, `use:enhance`, preloading
- [x] Vite plugin and asset manifest
- [x] `tern dev`: single port, Go restart, Vite HMR, error overlay
- [x] `tern build`: embedded single binary
- [x] `tern new` with a starter template
- [x] Example apps: the `tern new` starter and `examples/todos`, with end-to-end tests
- [x] Documentation: [guide](guide.md), [deployment](deployment.md), [architecture](ARCHITECTURE.md)

## v0.2: Server-side rendering

- [x] Embedded JavaScript engine for Svelte SSR (`modernc.org/quickjs`, no cgo; see ADR 0004)
- [x] Hydration instead of client-only mounting
- [x] Renderer pool with timeouts, memory limits and fallback to client rendering
- [x] Server rendering in development through Vite
- [x] Per-route `const SSR = false` and app-wide `kit.WithoutSSR()`

## v0.3: Performance and deployment

- [x] Streaming deferred data from loads (`kit.Defer`, rendered with `{#await}`)
- [x] Prerendering at build time (`const Prerender = true`, `Entries` for dynamic routes)
- [x] Deployment guide: Docker, systemd, Fly.io, reverse proxies ([docs/deployment.md](deployment.md))
- [x] Assets: Vite imports (`import logo from '$lib/logo.svg'`) and `static/`, with immutable caching

## v1.0: Stable

- [x] Plugin system and built-in plugins (health, request logging, compression, security headers)
- [x] Base paths (`tern.json`)
- [x] Public environment variables
- [x] Stability promise ([stability.md](stability.md))

## v1.1

- [x] More plugins: sessions, metrics, rate limiting, CORS
- [x] Service workers
- [x] Translations (`src/locales`, `t()`, `ctx.T`)
- [x] Responsive images (`?image`, `<Image>`)

## v1.2

- [x] Server rendering: memory leak fixed, worker cap, render cache ([ADR 0007](adr/0007-ssr-throughput-and-memory.md))
- [x] Web APIs for libraries during server rendering, and a compatibility suite checked against Node
- [x] Fuzz tests for routing and messages
- [x] Markdown pages and content collections
- [x] WebSocket and server-sent event helpers, with pub/sub
- [x] Background jobs, cron and database migrations
- [x] OAuth2 and OpenID Connect: sign-in with providers, a provider of your own, and roles ([ADR 0008](adr/0008-oauth-and-oidc.md))
- [x] Email with Svelte templates; file uploads and storage
- [x] OpenAPI from endpoints, with a typed client
- [x] A status page example
- [x] Experimental: `<script lang="go">`, in WebAssembly and live on the server ([notes](experimental-go-script.md))
- [x] Remote images for `<Image>`, translated URL paths, starter templates, a SvelteKit migration guide

## v1.3

- [x] Live components, production ready: per-component goroutines, typed markup with `tern check`, patches, pending and optimistic updates, bindings, events, several server processes through a broker
- [x] An MCP server covering all of Tern: project scaffolding, routes and their types, docs, codegen, and running checks, for AI coding tools
- [x] A component library: accessible, keyboard-friendly Svelte components with Tern's own design, themeable with CSS variables

## v1.4: Everything an app needs (released)

Built alongside a real app, a self-hosted web analytics tool, so every piece is used before it ships.

**Starting a project**
- [x] `npm create tern@latest`, and an interactive `tern new`: name, template, Tern UI, git, install
- [x] `ctx.ClientIP()` with trusted proxies, the Intl shim's `signDisplay`, a password sign-in recipe

**Tern UI: the complete set**
- [x] Overlays: Popover, Hover Card, Alert Dialog, Sheet, Drawer, Context Menu, Menubar
- [x] Inputs: Combobox, Command palette, custom Select, Slider, Toggle, Toggle Group, Input OTP, Input Group, Date Picker, Calendar, Label, Button Group, Password input
- [x] Layout and navigation: Sidebar, Navigation Menu, Breadcrumb, Pagination, Collapsible, Resizable, Scroll Area, Aspect Ratio, Carousel, Empty, Item, Table
- [x] Data: a Data Table with sorting, filtering, selection and pagination
- [x] Charts: line, area, bar and donut, with axes, tooltips, legends, and a table for screen readers
- [x] Blocks wired to Tern's plugins: sign-in and sign-up (passwords, `plugins/oauth`), the consent screen for `plugins/oidc`, and an app shell (`Sidebar`)
- [x] `tern ui add <component>`: copy a component into the project, to own and change it

**Validation**
- [x] Schemas in Go that validate actions and endpoints, generate matching TypeScript validators so the page checks as you type, and show each error on its `Field`

**The analytics app**
- [x] A tracking script and collection endpoint, rollups as jobs, retention, a live dashboard, weekly email reports, a stats API with OpenAPI, one binary

## v1.5: Every app's next questions

**Translations for everything built in**
- [ ] Tern UI's texts ("Show password", "Previous page", …) and the validation messages through `src/locales` and `t()`, in Go and in the browser, with English as the default

**Type rough edges**
- [ ] Go slices that are never nil, typed without `| null`: `nil` sent as `[]`, or a tag to say so
- [ ] Controls that post through hidden inputs, such as `Combobox`, checked as the user types inside a `Form`

**Accounts**
- [ ] Password reset and email verification, with email and jobs
- [ ] Passkeys (WebAuthn): sign-in without a password
- [ ] Signed-in devices: a list of sessions, and signing out of the others

## v1.6: Faster server rendering

[benchmarks/vs-sveltekit](../benchmarks/vs-sveltekit) shows renders are 1.8× to 8.5× slower than SvelteKit on Node, from QuickJS's speed per render and VMs that stop scaling at four.

- [ ] Profile a render to find what is Tern's own cost (payload encoding, shims, string building) and what is QuickJS's
- [ ] Prototype rendering in several processes of the one binary, and QuickJS in WebAssembly with wazero, and choose by measurement in an ADR
- [ ] Track the benchmarks in CI, so renders don't get slower unnoticed

## Next


