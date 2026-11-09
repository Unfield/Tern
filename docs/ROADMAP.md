# Roadmap

This is the planned order of work. It changes as we learn; the [issue tracker](https://github.com/Unfield/Tern/issues) is the live view.

## v0.1: Foundations (client-rendered)

The goal: you can build a real app with Tern and deploy it as one binary.

- [x] Route scanner and matcher (`slug.param`, `lang.optional`, `path.rest`, `app.group`, `id.param.matcher`)
- [ ] Manifest code generation for route packages
- [ ] `kit`: `Ctx`, `Load`, layouts with `Parent()`, `Redirect`/`Error`/`NotFound`
- [ ] Form actions with `Fail` and progressive enhancement
- [ ] Endpoints (`GET`, `POST`, … in a route's Go package)
- [ ] `Handle` hook and `net/http` middleware
- [ ] Go → TypeScript type generation, including layout data merging
- [ ] Client runtime: router, `goto`, `invalidate`, `page` state, `use:enhance`
- [ ] Vite plugin and asset manifest
- [ ] `tern dev`: single port, Go restart, Vite HMR, error overlay
- [ ] `tern build`: embedded single binary
- [ ] `tern new` with a starter template
- [ ] Documentation site and two example apps

## v0.2: Server-side rendering

- [ ] Embedded JavaScript engine for Svelte SSR (QuickJS via wazero, no cgo)
- [ ] Hydration instead of client-only mounting
- [ ] Renderer pool and benchmarks
- [ ] Optional Node sidecar renderer for maximum compatibility
- [ ] Per-route `ssr` / `csr` options

## v0.3: Performance and deployment

- [ ] Streaming deferred data from `Load`
- [ ] Prerendering of static routes at build time
- [ ] Deployment guides and adapters (Docker, Fly.io, serverless)
- [ ] Image and asset helpers

## Later

- i18n helpers
- Plugin system
- Service worker support

## Toward v1.0

v1.0 means a stable `kit` API and client API, complete documentation and a commitment to semantic versioning without breaking changes in minor releases.
