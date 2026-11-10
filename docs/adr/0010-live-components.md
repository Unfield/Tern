# 0010. Live components

- Status: Accepted
- Date: 2026-11-10

## Context

[ADR 0009](0009-experimental-go-script.md) introduced `<script lang="go">`. It compiles to WebAssembly, and a `live` variant runs the same API on the server. The measurements favored the live variant: no download, real per-request state, and access to the database and session.

It wasn't ready for production:

- **One global lock:** one reactive core behind a global lock, so one slow Func stalled every component of every visitor.
- **No types:** `svelte-check` failed on every Go script.
- **Whole values:** every state change sent the whole value.
- **Missing pieces:** no pending or error state, no bindings, no events to parents, no prop updates, no access control, no limits.
- **One process:** state was lost on reconnect, and nothing worked across several processes.

## Decision

- **Supported, without a flag.** `<script lang="go" live>` is a supported feature with the stability promise. WebAssembly mode stays experimental.
- **A goroutine per component.** Each component owns its reactive core and runs its work on its own goroutine, in order, like an actor.
  - **Shared values:** `Global` values fan out to components by posting to their goroutines, with a version so out-of-order updates are dropped.
  - **Other goroutines:** they go through `c.Do`.
  - **Calls:** a call answers after the updates it caused in its own component are sent.
- **Asynchronous endpoint.** The WebSocket handler never waits for a component: mounts, calls and bound values are queued on the component's goroutine. Calls on one connection are rate-limited, mounts are capped, and a reader that falls behind is disconnected.
- **Batches and patches.** The values changed in a batch go in one message. Lists and objects go as a small patch (set, delete, splice) when that is smaller than the value. The browser applies patches to Svelte's state in place.
- **Errors are opt-in to show.** `goscript.Fail` errors reach the page. Other errors and panics are logged and reported generically. `c.Reject` refuses a mount, because the WebSocket is as reachable as an endpoint.
- **Client-owned state is declared.** Only a `State` declared with `Bind` can be set from the page. Only `Bind` and `Restore` values survive reconnects, because they come back from the browser.
- **Types from Go.**
  - `tern gen` type-checks each live component's package and writes TypeScript types for its values and Funcs.
  - `tern check` runs `svelte-check` on a copy of `src` where each Go script is replaced by declarations of those types, keeping line numbers.
  - We chose this over a language-server plugin: it works with the stock `svelte-check` in CI and editors' "run check" tasks.
- **Several processes through a broker.** The broker interface is the same shape as `realtime.Broker`, and `realtime/pgbroker` implements it on PostgreSQL LISTEN/NOTIFY.
  - **`Shared`:** last-writer-wins values, using Lamport clocks.
  - **`Presence`:** per-process counts with heartbeats and expiry.
  - **`Publish`/`Subscribe`:** "something changed" messages.
  - **Consensus:** Tern doesn't try to be a consensus system. Data that must not lose writes belongs in the database.
- **Plugins' hooks run before the app's.** Live components read the user from `Locals`, set by app hooks that need the session plugin. The guide's own example needed this order.

## Consequences

- **Per-instance cost:** a mounted component costs about 6 KiB and a goroutine, and a call about a microsecond in process. Ten thousand open components fit in about 60 MiB.
- **A round trip per interaction:** this suits dashboards, admin tools, forms and collaborative features, not offline or frame-rate interactions.
- **Client-side navigation:** a component appears one round trip after mounting.
- **Breaking changes to the experimental API:**
  - `Global.Get` no longer subscribes; use the signal `Show` or `Use` returns.
  - `MountLive` takes options.
- **Hook order:** apps whose hooks relied on running before plugins' hooks see a different order.
