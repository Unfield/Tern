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

## Next

- A documentation website generated from `docs/`
- `beforeNavigate` / `afterNavigate` hooks and finer-grained `invalidate`
- Prerendering endpoints (for example `sitemap.xml`)

## Later

- i18n helpers
- Plugin system
- Service worker support

## Toward v1.0

v1.0 means a stable `kit` API and client API, complete documentation and a commitment to semantic versioning without breaking changes in minor releases.
