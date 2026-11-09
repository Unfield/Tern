# 0006. A small plugin interface on top of net/http

- Status: Accepted
- Date: 2026-11-09

## Context

Production apps need the same pieces again and again: health checks, access logs, compression, security headers, analytics tags. `kit.WithHook` and `kit.WithMiddleware` covered part of this, but a reusable package often needs several hooks at once (a route of its own, a tag in every page, work on shutdown), and users should add it with one line.

## Decision

- A plugin is a Go value with `Name() string` and `Setup(*kit.Setup) error`, added with `kit.WithPlugins`.
- `Setup` can register hooks, middleware, matchers, `http.Handler`s that are served before routes (with `net/http.ServeMux` patterns), HTML for every page's `<head>` (with the request's CSP nonce), and shutdown functions.
- Plugins are set up in order after the other options; a name can be added once; a `Setup` error stops the app from starting.
- Built-in plugins live in `plugins/` in the main module and use only the standard library.
- Client-side extension stays with Vite: Tern's Vite plugin composes with any other Vite plugin.

## Consequences

- Extending Tern means writing ordinary Go against `net/http`; there is no separate plugin runtime or registry.
- Plugins cannot change routing or rendering internals. That keeps the public surface small enough to promise stability for at 1.0, and can be extended later without breaking existing plugins.
