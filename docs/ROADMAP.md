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
- [ ] Background jobs, cron and database migrations
- [ ] OAuth2 and OpenID Connect: sign-in with providers, and a provider of your own
- [ ] Email with Svelte templates; file uploads and storage
- [ ] OpenAPI from endpoints, with a typed client
- [ ] A status page example
- [ ] Experimental: `<script lang="go">`

## Next

- Remote images for `<Image>`
- Translated URL paths
