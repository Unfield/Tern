# 0004. Server-side rendering with embedded QuickJS

- Status: Accepted
- Date: 2026-11-09

## Context

Svelte's server output is JavaScript, so rendering pages on a Go server needs a JavaScript engine. [ADR 0002](0002-client-rendering-first.md) deferred the choice to v0.2. The requirements: no cgo (Tern apps cross-compile to a single static binary), full support for the modern JavaScript Svelte 5 emits, safe limits on time and memory, and predictable performance.

We bundled a real Svelte 5 component tree for the server and ran it in the pure-Go candidates:

| Engine | Result |
|---|---|
| `github.com/dop251/goja` | Fails to parse Svelte 5's runtime (`SyntaxError: Unexpected reserved word`) |
| `modernc.org/quickjs` (QuickJS translated to Go) | Renders correctly; about 12 ms to load the bundle, about 0.2 ms per render |

A Node sidecar would be fully compatible but brings back the Node dependency in production that Tern exists to remove.

## Decision

- Production renders use **`modernc.org/quickjs`** through a pool of VMs (`internal/ssr`). The bundle is compiled to bytecode once; each VM loads it on creation.
- Each render has a timeout (5 s) and each VM a memory limit (256 MiB). A VM that times out or throws is discarded and replaced.
- QuickJS implements only the language, so the server bundle includes small polyfills (`URL`, `URLSearchParams`, `queueMicrotask`, and `console`, which logs through the app's `slog` logger).
- Vite builds the renderer as one self-contained IIFE (`vite build --ssr`), embedded in the binary next to the client.
- In development, the Go app asks Vite to render (`ssrLoadModule`), so server rendering always reflects the latest source.
- The server and the client choose layouts, pages and error boundaries with the same code (`runtime/select.ts`), so hydration matches.
- If a render fails, the page falls back to client rendering and the error is logged. A rendering bug degrades one page instead of returning errors.
- Pages opt out with `const SSR = false` in their route package; apps with `kit.WithoutSSR()`.

## Consequences

- Pages work without JavaScript, are crawlable, and paint sooner.
- Binaries grow by about 3 MB.
- Server-side component code cannot use browser or Node APIs beyond the polyfills. This matches Svelte's guidance to use them only in `onMount` and effects, which do not run on the server.
- Throughput is bounded by the pool size (GOMAXPROCS by default, configurable with `kit.WithSSRWorkers`).
