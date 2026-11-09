# 0005. Selective reloading of layout data

- Status: Accepted
- Date: 2026-11-09

## Context

On a client-side navigation Tern fetched `__data.json`, which reran every load for the target route: the root layout, any nested layouts and the page. In apps with a sidebar or header that queries a database, every click repeated those queries even though their data had not changed. SvelteKit avoids this by tracking what each load uses. Tern's loads are Go functions, so the tracking has to happen on the server, while route matching stays the server's job (the client does not know which route a URL belongs to).

## Decision

- `kit.Ctx` records what a load reads: `ctx.Param(name)`, `ctx.URL()`, `kit.Parent`, and keys declared with `ctx.Depends`. Each node's usage is sent with its data.
- The client keeps each layout's data with its usage and the parameter values it read. On navigation it sends `x-tern-reuse`: the layouts it holds whose inputs, as far as it can tell, are still valid (not depending on a changed URL or an invalidated key).
- The server reuses a layout only if every parameter it read has the same value for the new URL. Reuse is a prefix of the layout chain: when one layout reruns, the ones below it rerun too. A skipped layout still runs, on demand, if a child's `kit.Parent` needs its data.
- Pages always rerun. Full page loads, form actions and `invalidateAll` run every load.
- `invalidate(key)` drops layouts that depend on `key` from the reuse list.

## Consequences

- Navigating under a layout no longer repeats its queries; the dashboard example's sidebar shows `reused: [0, 1]` on every navigation.
- Reading `ctx.Request` directly is not tracked. The guide says to use `ctx.URL()` or `ctx.Depends` for anything a layout's result depends on.
- The client sends a small JSON list with each data request; the server never stores per-client state.
