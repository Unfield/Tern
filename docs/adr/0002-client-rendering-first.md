# 0002. Client-side rendering first, SSR in v0.2

- Status: Accepted; implemented in v0.2 by [0004](0004-ssr-with-embedded-quickjs.md)
- Date: 2026-11-09

## Context

Server-side rendering matters for SEO and first paint, and is a core goal. But Svelte's server output is JavaScript, so SSR from Go needs an embedded JS engine (QuickJS via wazero, goja or V8 via cgo) or a Node sidecar. Each option has performance, packaging and compatibility tradeoffs that are best evaluated once the rest of the framework exists.

## Decision

v0.1 renders on the client: Go runs `Load` and returns an HTML shell with the data inlined as JSON, and the client mounts the components. SSR arrives in v0.2.

To keep that transition free for users, v0.1 must follow these rules:

1. `Load` and actions return **data only**; they never produce markup.
2. The request pipeline has a single rendering step behind an interface, so an SSR renderer can replace the shell renderer.
3. The client runtime is written so that it can **hydrate** as well as mount.

## Consequences

- v0.1 is much smaller and can ship sooner.
- v0.1 pages are not crawlable without JavaScript and have a slower first paint; this is documented.
- Moving to v0.2 requires no changes to application code.
