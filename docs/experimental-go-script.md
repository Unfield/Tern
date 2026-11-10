# Experimental: `<script lang="go">`

> Status: experimental. The API and the build can change in any release. Enable it with `"experimental": { "goScript": true }` in `tern.json`.

A Svelte component can have its logic in Go instead of TypeScript. The markup stays Svelte. The Go code runs in the browser as WebAssembly, with Go versions of Svelte's runes.

```svelte
<script lang="go">
import "github.com/Unfield/Tern/goscript"

func Setup(c *goscript.Component) {
	count := goscript.State(c, "count", 0)
	goscript.Derived(c, "doubled", func() int { return count.Get() * 2 })
	c.Func("increment", func() { count.Set(count.Get() + 1) })
}
</script>

<button onclick={increment}>{count} × 2 = {doubled}</button>
```

See [examples/goscript](../examples/goscript) for a working app.

## The API

| Go | Svelte equivalent | In the markup |
|---|---|---|
| `goscript.State(c, "name", initial)` | `let name = $state(initial)` | `{name}` |
| `goscript.Derived(c, "name", func() T)` | `let name = $derived(...)` | `{name}` |
| `goscript.Effect(c, func())` | `$effect(...)` | |
| `c.Func("name", fn)` | a function | `onclick={name}` |
| `c.Props(&props)` | `$props()` | `<Counter start={10} />` |

- **Signals:** `Get` reads a signal and subscribes the running `Derived` or `Effect` to it. `Peek` reads without subscribing. `Set` and `Update` write.
- **Batching:** changes made in one `Func` call are applied together, so a derived value recomputes once.
- **Arguments:** `Func` arguments are decoded from the JavaScript values passed to them. DOM events arrive as a `goscript.Event` with `Value`, `Checked` and `Key`. A `Func`'s return value goes back to JavaScript.

## How it works

1. **Scanning.** When Vite starts (`tern dev`) or builds (`tern build`), Tern finds every `.svelte` file with a `<script lang="go">`. It writes each script as its own Go package under `.tern/goscript/`, and a `main` package that registers them all. `//line` directives make Go's compile errors point at the `.svelte` file and line.
2. **Initial state.** That program runs once natively (`go run`), mounting every component with empty props and recording its state. The result is the **initial state**, so server rendering and the first paint show real values instead of empty ones.
3. **WebAssembly.** The same program is built for `GOOS=js GOARCH=wasm` into one module for the whole app. It uses the Go toolchain's matching `wasm_exec.js`.
4. **Bindings.** A Svelte preprocessor replaces the Go script with a few lines of TypeScript:
   - a `useGo()` call
   - one `$derived` per state name, reading the values Go publishes
   - one function per `Func`
5. **In the browser.** The module loads once per page. Each component instance mounts its own Go instance, which publishes values into Svelte state through a small bridge. Calls and events go the other way as JSON.

## What it costs

Measured in the example app, in Chrome on an Apple M4:

| | Go (WebAssembly) | TypeScript |
|---|---|---|
| Counting primes to 5 million (sieve), median of 15 warm runs | 23.2 ms | 15.6 ms |
| A click through the bridge: Go call, state change and Svelte update | 0.04 ms | |
| Download | 4.5 MB, 1.26 MB gzipped | part of the normal bundle |
| Build time added | about 1.5 s (`go run` plus `go build`) | |

**Speed:** WebAssembly compiled by the standard Go toolchain is **not faster** than JavaScript for this kind of code. V8's JIT optimizes a simple loop over a `Uint8Array` very well, while Go's WebAssembly output has bounds checks, a larger runtime and no SIMD by default. The bridge is fast enough to be invisible in UI code.

**Size:** the module is big because it carries the Go runtime, garbage collector, `reflect` and `encoding/json`. It is paid once per page, but it is still about ten times a typical Svelte page's JavaScript.

## Limitations

- **Props:** they are read when a component mounts. Changing them afterwards remounts the Go instance.
- **Initial state:** it is computed with empty props. Values that depend on props appear once the module loads.
- **Shared module:** all Go components share one module. A component with a Go script can't also have a TypeScript script.
- **Tooling:** `svelte-check` doesn't understand Go scripts. Run `go vet` on `.tern/goscript` to check them.
- **SSR timing:** server rendering shows the build-time initial state, not state computed for each request.

## Where this could go

The experiment shows what Go in components is good for, and what it isn't:

- **Not for speed.** For typical UI logic, TypeScript in Svelte is faster to load and to run. A future switch to [TinyGo](https://tinygo.org) could shrink the module to a few hundred kilobytes, but would not make tight loops faster than V8.
- **For sharing Go code.** The real value is that the same Go runs on the server and in the browser. Validation rules, pricing, permissions and parsers can be written once, with Go's types and libraries, without a TypeScript copy drifting out of sync. The build-time initial state is a first taste of that.
- **The most promising next step is server-run components** ("live" Go components). The same `goscript` API would run on the server, over a WebSocket, instead of in WebAssembly. This is LiveView's model, and the API already allows it, since components only see `State`, `Derived`, `Effect` and `Func`.
  - **No download:** the browser loads no module at all.
  - **Direct access:** state lives next to the database and the user's session.
  - **Plays to Go's strengths:** many cheap concurrent connections.

  It fits Tern's realtime package, and keeps "Go on the server" honest.

See [ADR 0009](adr/0009-experimental-go-script.md) for the decision.
