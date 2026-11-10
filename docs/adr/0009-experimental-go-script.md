# 0009. Experimental `<script lang="go">`

- Status: Accepted (experimental)
- Date: 2026-11-10

## Context

Writing component logic in Go was requested, with runes, possibly to replace TypeScript for performance. Two ways to run Go for a component:

- **WebAssembly in the browser.**
- **Go on the server, connected to the page.**

We wanted to learn what each costs before committing to an API.

## Decision

- **Behind a flag.** `<script lang="go">` is experimental, enabled with `experimental.goScript` in `tern.json`.
- **WebAssembly first.** The first version compiles to WebAssembly with the standard Go toolchain, because it needs no server connection and works with prerendered pages.
- **Runes as a Go package.** The `goscript` package provides `State`, `Derived`, `Effect` and `Func`, and its synchronous reactive core batches changes per call.
  - Components only use this API, so the same code could later run on the server instead.
- **Build-time initial state.** `Setup` runs natively at build time, so server rendering shows real state without a JavaScript engine running Go.
- **One module per app.** Each component is a Go package, and all of them are linked into a single WebAssembly module.

## Consequences

- The measurements in [docs/experimental-go-script.md](../experimental-go-script.md) show that Go WebAssembly is slower than V8 for tight loops (23 ms vs 16 ms on a prime sieve) and costs a 1.3 MB download. Performance is not a reason to use it.
- The value is sharing Go between server and browser.
- Live components (`<script lang="go" live>`) run the same API on the server over one WebSocket per page, with real per-request server rendering and shared `Global` state. They need no download, so they are the more practical of the two modes for most apps.
- The flag stays until that design is settled. The API may change.
