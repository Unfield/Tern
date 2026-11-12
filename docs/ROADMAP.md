# Roadmap

This is the planned order of work, released and to come. It changes as we learn; the [issue tracker](https://github.com/Unfield/Tern/issues) is the live view.

Every release keeps the [stability promise](stability.md): apps written for v1.0 keep working, and new features arrive as additions you opt into. A release ships with its docs, tests and an example of the feature in use.

The plan, at a glance:

| Version | Theme |
|---|---|
| [v1.9](#v19-logging-security-signals-and-deploying-released) | Logging, security signals, `tern deploy`, tern.land (released) |
| [v1.10](#v110-production-confidence-released) | Production confidence: tracing, error reports, test helpers (released) |
| [v1.11](#v111-teams-and-billing-released) | Teams and billing, for SaaS apps (released) |
| [v1.12](#v112-rendering-on-demand-and-caching-released) | Rendering on demand, and caching (released) |
| [v1.13](#v113-native-rendering-phase-3-released) | Native rendering, phase 3: no JavaScript engine for most apps (released) |
| [Later](#later) | Search, feature flags, admin screens, AI, editor support, hosting |

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

## v1.5: Native rendering (released)

[benchmarks/vs-sveltekit](../benchmarks/vs-sveltekit) showed renders 1.8× to 8.5× slower than SvelteKit on Node, from Svelte's code running in QuickJS. [ADR 0011](adr/0011-native-rendering.md) has the design and how it turned out.

- [x] Profile a render, and a hand-written spike with the same HTML as Svelte
- [x] A compiler from the server build's modules (components, Tern's and Svelte's runtimes, libraries) to Go, with what it can't compile left to the JavaScript engine
- [x] A Go model of JavaScript values with QuickJS's semantics: shapes and inline caches, realms that render in parallel, generators, typed arrays, Proxy; conformance tests against Node
- [x] Intrinsics for Svelte's hottest helpers, used only while their source is unchanged
- [x] `tern build` compiling it and reporting what isn't native; fallback to the engine for anything that fails
- [x] Every example rendering natively with the engine's HTML, checked with `TERN_NATIVE_VERIFY=1`
- [ ] Typed fast paths from the loads' Go structs (phase 2)
- [x] Smaller binaries for large apps, and the vs-sveltekit benchmark tracked in CI: function descriptors as static data, no server bundle without the engine (dashboard 38.9 → 37.4 MB); each run on `main` compared with the last, with a warning when Tern is 10% slower or its binary 5% larger

## v1.6: Fully native rendering, and accounts (released)

**Native rendering, complete**
- [x] Compile what was left to the JavaScript engine: dynamic `import()`, BigInt, async generators and `for await`, `new.target`
- [x] Every built-in QuickJS has: grouping, Set methods, iterator helpers, `WeakRef`, the RegExp `d` and `v` flags, and the rest
- [x] A page's own errors keep it native; only what native rendering can't do sends a page to the engine
- [x] Less work per render: the payload says when it has deferred values, and the result isn't encoded as JSON

**Translations for everything built in**
- [x] Tern UI's texts ("Show password", "Previous page", …) and the validation messages through `src/locales` and `t()`, in Go and in the browser, with English as the default

**Type rough edges**
- [x] Go slices that are never nil, typed without `| null`: `nil` sent as `[]`, or a tag to say so
- [x] Controls that post through hidden inputs, such as `Combobox`, checked as the user types inside a `Form`

**Accounts**
- [x] Password reset and email verification, with email and jobs
- [x] Passkeys (WebAuthn): sign-in without a password
- [x] Signed-in devices: a list of sessions, and signing out of the others
- [x] Tern UI screens for each, and [examples/accounts](../examples/accounts)

## v1.7: Faster native rendering, and accounts for real apps (released)

**Native rendering**
- [x] Page data encoded by Tern's own JSON encoder, without encoding/json or a copy for nil slices: 6× faster, uncached renders 17% faster
- [x] Smaller binaries for large apps: 11% from compiling the renderer without inlining, at the same speed
- [x] The vs-sveltekit benchmark run in CI, and every change's renders compared with the commit before

**Accounts**
- [x] Two-step sign-in with an authenticator app (TOTP), and recovery codes
- [x] Sign in with GitHub, Google or OpenID Connect into an accounts user
- [x] Changing the email address, confirmed through the new one
- [x] Sign-in, sign-up and reset attempts limited per address and per IP, built in

**CLI**
- [x] `tern routes -json`
- [x] `tern dev` says which routes were added and removed

## v1.8: A full app to start from, and native rendering phase 2 (released)

**Starting a project**
- [x] `tern new` asks for a bare app or a full one: the `saas` template, with accounts, a dashboard, settings and per-user data
- [x] `tern dev` reads `.env`

**Native rendering, phase 2** ([ADR 0013](adr/0013-native-rendering-phase-2.md))
- [x] Page data handed to the renderer as Go values, without parsing JSON in the realm
- [x] Svelte's Renderer output and its attribute helpers in Go, checked against Node
- [x] The document written in one buffer
- [x] Typed fast paths from the loads' Go structs: profiled and set aside, as inline caches already make property reads cheap

## v1.9: Logging, security signals, and deploying (released)

**Logging** ([ADR 0014](adr/0014-logging.md))
- [x] Structured logging on `log/slog` with request IDs, trace context, redaction and sampling
- [x] Sinks for OpenTelemetry (OTLP), Loki, Datadog and rotating files
- [x] A live log view in `tern dev`

**HTTPS and security signals** ([ADR 0015](adr/0015-security-signals.md))
- [x] HTTPS served by the app itself, with certificates from Let's Encrypt
- [x] `plugins/signals`: JA4, HTTP/2 and header-order fingerprints, a score with reasons, a client check with a proof of work, device IDs
- [x] Rate limits per client fingerprint, and emails about sign-ins from new devices

**Deploying** ([ADR 0016](adr/0016-tern-deploy.md))
- [x] `tern deploy` to a server over SSH, with zero-downtime restarts, releases, rollbacks and logs
- [x] To Fly.io, and to container registries with the platform's deploy command after
- [x] Providers of your own, as `tern-deploy-<name>` programs

**tern.land**
- [x] The website rebuilt as a Tern app, every page prerendered: a landing page, the docs with search, the decision records and the examples
- [x] A styled first paint under `tern dev`

## v1.10: Production confidence (released)

Knowing what an app does in production, and that it still does it after a change.

**Tracing** ([ADR 0017](adr/0017-tracing.md))
- [x] OpenTelemetry spans for each request, and the loads, actions, renders, SQL queries, jobs and outgoing HTTP calls in it, exported over OTLP next to the logs
- [x] A waterfall of each request in `tern dev`'s log view
- [x] Trace context passed on to outgoing requests and jobs, so a trace follows work across services

**Error reports** ([ADR 0018](adr/0018-error-reports.md))
- [x] Unexpected errors and panics grouped by cause, with stack traces, the request and the user, and sent to Sentry-compatible services
- [x] Browser errors reported to the server the same way
- [x] A list in `tern dev`'s log view, as they happen

**Testing** ([docs/testing.md](testing.md))
- [x] `kit/kittest`: call loads, actions and endpoints from Go tests, as a signed-in user, with a test database that resets
- [x] Render a page in a test and query its HTML
- [x] Browser tests in the templates, with Playwright set up and an example test

## v1.11: Teams and billing (released)

What the SaaS template still leaves to you.

**Teams**
- [x] Organizations in `accounts`: members, invitations by email, roles with `rbac`, and switching between teams
- [x] An audit log of account and team events, for users and for support
- [x] Tern UI screens for team settings, members and invitations

**Billing**
- [x] A billing plugin for Stripe: Checkout, the customer portal, plans and subscriptions, trials, and seats per team
- [x] Webhooks verified and processed as jobs, so a slow or repeated event is safe
- [x] Limits per plan, checked in loads and actions
- [x] Pricing and billing screens in Tern UI, and both in the saas template

## v1.12: Rendering on demand and caching (released)

Pages as fast as prerendered ones, with data that changes.

- [x] Incremental prerendering: a page is rendered on its first request, served from the cache after, and rendered again after a time or when its data changes
- [x] Tags on loads and pages, and `kit.Revalidate(tag)` to refresh everything that used some data
- [x] A cache for load results, in memory, on disk or in Redis, shared by every process
- [x] Cache headers per route, for CDNs in front of the app
- [x] Pages with nothing interactive can ship no JavaScript at all

## v1.13: Native rendering, phase 3 (released)

Every page rendered by Go, and smaller binaries.

- ~~A value representation without boxing numbers and strings~~: profiled and decided against, as it would gain about a tenth for a rewrite of the runtime ([ADR 0022](adr/0022-keep-boxed-values.md))
- [x] `tern build` leaves the JavaScript engine out of apps that don't need it, for smaller binaries
- [x] Native rendering for emails and live components too

## v2.0: Leaner native rendering (planned)

Faster pages that can't come from a cache, smaller binaries and less memory, each change measured with `benchmarks/compare.sh` and the Tern vs SvelteKit run in CI.

**Uncached renders** (`/items` takes about 250 µs, most of it calls between compiled functions and allocating values)

- [ ] Reuse the arguments slices of calls, where the callee doesn't keep them (`arguments`, rest parameters and `bind` copy first)
- [ ] Direct calls between compiled functions when the callee is known at compile time, without the generic call protocol
- [ ] Revisit unboxed values ([ADR 0022](adr/0022-keep-boxed-values.md)) only with a profile from a real app that shows boxing as the main cost

**Binary size** (the dashboard example: 37.4 MB, 11 MB of it the compiled renderer)

- [ ] Generated code that shares more: common call and property-access sequences as runtime helpers instead of inline code
- [ ] Static assets kept only compressed, decompressed for the rare client that doesn't accept gzip

**Memory**

- [ ] Less memory on data-heavy pages (about 110 MB resident for the items benchmark, with four workers): a heap profile of where it goes, and the biggest uses cut

## v1.16: Secure by default, and typed SQL (in progress)

- [x] Security headers, a Content-Security-Policy and a rate limit on writes, on by default ([docs/security.md](security.md))
- [x] Known vulnerabilities in Go modules and npm packages, in `tern doctor` and CI
- [x] Routes closed by default: each route with server code says who may use it ([ADR 0027](adr/0027-secure-by-default.md))
- [x] Typed SQL from `queries/*.sql`, checked against the schema ([ADR 0028](adr/0028-typed-sql.md))
- [ ] Checks for risky migrations
- [ ] An HTTP client for outgoing requests that refuses internal addresses
- [ ] Settings and secrets declared once, checked at start and kept out of logs
- [ ] Encrypted fields for personal data
- [ ] Safer uploads: types checked against contents, size limits, signed links

## Later

Planned, in no fixed order yet. Tell us in the issues which you need first.

**Building**

- [x] Fewer allocations in native renders: boxes for HTML and page data from slabs, 40% fewer allocations on the 100-row table ([ADR 0022](adr/0022-keep-boxed-values.md#since))
- [ ] A `tern build` that needs only Go, with Svelte's compiler compiled to Go and esbuild for bundling ([ADR 0026](adr/0026-svelte-compiler-in-go.md))
- [ ] CommonJS modules in native rendering
- [ ] `fn.toString()` with the bundle's text, so verify mode finds no difference at all
- [x] Full-text search for content collections and app data, on SQLite or PostgreSQL, with a search box in Tern UI ([docs/search.md](search.md))
- [x] Feature flags and gradual rollouts, decided on the server, per user or team ([docs/flags.md](flags.md))
- [x] Dev tools in `tern dev`: a panel with each page's route, files, data, spans and logs, and errors over the page with their source ([docs/dev-tools.md](dev-tools.md))
- [x] `tern doctor`: checks before a deploy, of tools, versions, code generation, native rendering, migrations and the target's settings ([docs/deployment.md](deployment.md#before-you-deploy-tern-doctor))
- [x] Operations screens in the admin panel: users, the audit log, jobs with Retry, and error reports; actions on records ([docs/admin.md](admin.md#operations))
- [x] Admin screens generated from Go structs: lists, filters and forms for an app's own data ([docs/admin.md](admin.md))
- [x] Presence and shared state for live components, for collaborative screens ([docs/live-components.md](live-components.md#collaboration))

**AI**
- [x] Streaming endpoints for model responses, with typed partial results on the client ([guide](guide.md#streaming-endpoints))
- [x] An app's tools served over MCP, so assistants can use the app with the user's permissions ([docs/mcp-server.md](mcp-server.md))

**Tooling**
- [x] An editor extension: jump between a page and its Go load, see routes, and get `tern check` errors on save, for VS Code and Zed ([docs/editors.md](editors.md))
- [x] `tern add` for plugins, and a list of community plugins on tern.land ([tern.land/plugins](https://tern.land/plugins))

**Hosting**
- [ ] Hosting platforms as `tern deploy` providers, with what they add: previews per branch, logs and traces in one place, and managed databases. Tern stays deployable anywhere; no feature depends on a platform

**Learning**
- [x] A tutorial on tern.land that builds a complete app, one step per page
- [ ] A showcase of apps built with Tern
