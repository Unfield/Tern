# Live components

A live component is a Svelte component whose logic is Go, running on the server. The markup stays Svelte. The page talks to the server over one WebSocket, so state lives next to your database and session, and nothing extra is downloaded.

```svelte
<script lang="go" live>
import "github.com/Unfield/Tern/goscript"

func Setup(c *goscript.Component) {
	count := goscript.State(c, "count", 0)
	goscript.Derived(c, "doubled", func() int { return count.Get() * 2 })
	c.Func("increment", func() { count.Update(func(n int) int { return n + 1 }) })
}
</script>

<button onclick={increment} disabled={increment.pending}>{count} × 2 = {doubled}</button>
```

Live components need no flag. The same Go compiled to WebAssembly (without `live`) is [still experimental](experimental-go-script.md).

See [examples/goscript](../examples/goscript) for todos, presence and a shared counter, and the [status page](../examples/status/src/lib/LiveMonitors.svelte) for a dashboard that follows the monitor engine.

## When to use them

- **Good for:**
  - dashboards and admin tools
  - forms that validate against the database
  - collaborative and realtime features
  - anything that needs the session or server-side state
- **Not for:**
  - interactions that must work offline or respond within a frame, such as dragging or drawing
  - pages people read without interacting, which don't need a connection
- **Mixing is fine:** write those parts in TypeScript, in the same page or the same component's parents.

## The API

| Go | Like Svelte's | In the markup |
|---|---|---|
| `goscript.State(c, "name", initial)` | `$state` | `{name}` |
| `goscript.Derived(c, "name", func() T)` | `$derived` | `{name}` |
| `goscript.Effect(c, func())` | `$effect` | |
| `c.Func("name", fn)` | a function | `onclick={name}` |
| `goscript.Props[T](c)` | `$props()` | a signal that follows the parent's props |
| `c.Emit("saved", v)` | a callback prop | `<Child onsaved={(v) => …} />` |

### Signals and batching

- **Reading and writing:** `Get` reads a signal and subscribes the running `Derived` or `Effect` to it. `Peek` reads without subscribing. `Set` and `Update` write.
- **Batching:** one `Func` call is one batch. Every value it changes reaches the page in one message, after derived values have settled.
- **Immutability:** treat values as immutable. Set a new slice or map rather than changing the one a signal holds.

### Funcs

```go
c.Func("save", func(ctx context.Context, title string) (Post, error) {
	if title == "" {
		return Post{}, goscript.Fail("Give it a title.")
	}
	return store.Save(ctx, title)
})
```

- **Arguments:** decoded from JSON. A DOM event arrives as a `goscript.Event` with `Value`, `Checked` and `Key`.
- **Context:** a leading `context.Context` is canceled when the component goes away.
- **Results:** a Func may return nothing, a value, an error, or a value and an error. The value resolves the call's promise in the page.
- **Errors:** an error made with `goscript.Fail` is shown to the page. Any other error is logged and reported as "the component failed", so database errors and other internals don't leak.
- **Panics:** a panic is recovered and logged. The component keeps running.

In the markup, each Func is a function with extras:

| | |
|---|---|
| `save(args…)` | calls it; returns a promise |
| `save.pending` | `true` while a call waits for the server |
| `save.error` | the last call's `Fail` message, or `null` |
| `save.optimistic({ title: 'Draft' }, args…)` | shows these state values until the server answers |
| `save.debounce(300)` | a version for `oninput` that calls once typing pauses |

### `live`

Every live component's markup also has `live`:

| | |
|---|---|
| `live.connected` | whether the page is connected; show a "reconnecting" note when it isn't |
| `live.pending` | whether any call is waiting |
| `live.error` | why the component couldn't mount, such as a `Reject` message |
| `live.bind.name` | for `bind:value={live.bind.query}`, with a State declared with `goscript.Bind` |
| `live.set('name', value)` | sets a bindable State |
| `live.props` | everything the parent passed, such as `children` and event handlers |

The Go script is the component's instance script. To import other components or helpers for the markup, use a `<script module lang="ts">` block.

### Bindings

```go
query := goscript.State(c, "query", "", goscript.Bind)
goscript.Derived(c, "results", func() []Hit { return search(query.Get()) })
```

```svelte
<input bind:value={live.bind.query} />
```

- **Optimistic display:** the input shows what was typed at once. The server's value replaces it when the update is answered.
- **Opt-in:** only a State declared with `goscript.Bind` can be set from the page. Treat its value like any other user input.

### Background work

`Setup`, Funcs, Derived values and Effects run on the component's own goroutine, so they never race with each other. For anything else:

```go
c.Go(func(ctx context.Context) {        // a goroutine; ctx ends with the component
	for ev := range subscribe(ctx) {
		c.Do(func() { latest.Set(ev) })  // back on the component's goroutine
	}
})
c.Every(5*time.Second, func() { now.Set(time.Now()) })
c.OnDestroy(func() { … })
```

- **Rendering:** `c.Rendering()` is true while `Setup` runs only to render the page on the server. `Effect`, `Go`, `Every`, `Do` and `Presence.Join` do nothing then.

### Access control

A live component is reachable by anyone who can open `/_tern/live`, like an endpoint, so check access in `Setup`:

```go
admin, ok := c.Locals()["admin"].(app.Admin)
if !ok {
	c.Reject("Sign in to see this.")
}
```

- **Locals:** `c.Locals()` holds what your hooks set for the WebSocket request, such as the signed-in user from the session. Plugins' hooks run first, so sessions are available.
- **`Reject`:** stops `Setup` and sends only the message, which the markup shows with `live.error`. It works in the server render too.

## Sharing state

| | Scope | Use it for |
|---|---|---|
| `goscript.Global(v)` | this process | counters, caches, anything when you run one process |
| `goscript.Shared("name", v)` | every process, through the broker | small values everyone sees; the last write wins |
| `goscript.Presence("name")` | every process | how many are here: `room.Join(c)`, `room.Show(c, "online")` |
| `goscript.Publish(ctx, topic, v)` with `goscript.Subscribe(c, topic, fn)` | every process | "something changed": reload from the database |

- **Signals:** `Show(c, "name")` shows a shared value in the markup and returns a signal for Derived values. `Use(c)` returns the signal without showing it.
- **The usual pattern for data:** a Func writes to the database and then publishes. Every subscribed component reloads what it shows. The database stays the source of truth.

### Several server processes

By default these reach the components in one process. To run several instances of the app, connect them with a broker:

```go
b, err := pgbroker.New(ctx, os.Getenv("DATABASE_URL")) // github.com/Unfield/Tern/realtime/pgbroker
app, err := ternApp(kit.WithLiveBroker(b))
```

- **The broker:** `pgbroker` uses PostgreSQL's LISTEN/NOTIFY, so no other service is needed. Any `realtime.Broker`, such as one on Redis or NATS, works too.
- **`Shared`:** the last write wins everywhere. Keep counts that must not lose updates in the database.
- **`Presence`:** each process reports its count every 10 seconds. A process that stops reporting drops out after 30.
- **Sticky sessions:** not needed. A page that reconnects to another process remounts there.

## Connections, reconnects and deploys

- **One connection per page:** each page has one WebSocket for all its live components, whatever their nesting. Each instance runs on its own goroutine, so a slow Func delays only its own component.
- **Reconnecting:** when the connection drops, the page reconnects with a jittered backoff and remounts every component, keeping what it shows meanwhile.
  - **Calls in flight:** calls that were in flight fail with "connection lost".
  - **Calls made offline:** calls made while disconnected are sent after reconnecting.
- **Kept values:** a State declared with `goscript.Bind` or `goscript.Restore` keeps the value the page shows across reconnects, including to a new process after a deploy. Everything else starts again from `Setup`. Restored values come from the browser, so treat them as user input.
- **Graceful shutdown:** the server closes live connections with "going away". Pages reconnect to the new process.

## Server rendering and navigation

- **First load:** `Setup` runs during the server render with the request's props and `Locals`, so the page arrives with real state. That state is embedded in the page, so hydration starts from it. Pages with live components are not put in the render cache.
- **Client-side navigation:** there is no server render, so a live component's markup appears once its first state arrives, one round trip after mounting.
- **`live.loaded`:** tells you whether the state has arrived.

## Updates on the wire

- **Batches:** a batch sends one message with the values that changed.
- **Patches:** when a value is a list or an object and a patch is smaller than the value, a patch is sent instead. Appending to a 1,000-item list sends the new item, not the list.
- **Applying patches:** they are applied to Svelte's state in place, so only the changed items re-render.

## Types: `tern check`

`svelte-check` can't read Go, so use `tern check`, which the starter templates' `npm run check` runs:

```
$ go tool tern check
src/lib/Todos.svelte:31:12 Error: Property 'toFixed' does not exist on type 'string'.
src/lib/Todos.svelte:40:30 Error: Expected 1 arguments, but got 0.
```

- **What it does:** `tern gen` type-checks each live component's Go and writes TypeScript types for its State, Derived and Show values and its Funcs. `tern check` then runs `svelte-check` with each Go script replaced by declarations of those types.
- **What it catches:** typos, wrong fields and wrong arguments in the markup, at their real line.
- **Options:** any `svelte-check` option, such as `--fail-on-warnings`, can be passed after `tern check`.

## Testing

Test components in Go, without a browser, with `goscript/livetest`. Put the test in the app's `main` package, where `tern.gen.go` registers the components:

```go
func TestTodos(t *testing.T) {
	c := livetest.Mount(t, "src/lib/Todos.svelte", nil, livetest.WithLocals(map[string]any{"user": "ada"}))
	if err := c.Set("draft", "write docs"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Call("add"); err != nil {
		t.Fatal(err)
	}
	todos := livetest.Get[[]Todo](c, "todos")
	// ...
}
```

`Call` returns once the changes it caused are visible. `Eventually` waits for changes from other goroutines, such as `Every` or another component's `Global`.

## Limits and tuning

`kit.WithLive(kit.LiveOptions{…})`:

| Option | Default | |
|---|---|---|
| `MaxInstances` | 256 | live components one page may mount at once |
| `Rate` | 50 per second | calls and bound values one page may send, with bursts of twice that |
| `SendBuffer` | 256 messages | how far a slow browser may fall behind before it's disconnected (it reconnects fresh) |
| `PingInterval` | 25 s | keeps idle connections open through proxies |
| `Origins` | the app's own | other origins allowed to connect |

## What it costs

Measured on an Apple M4, in process (the WebSocket's own cost comes on top):

| | |
|---|---|
| Memory per mounted component | about 6 KiB (2.3 KiB heap, 4 KiB goroutine stack): 10,000 open components take about 60 MiB |
| A Func call with a state change and its emitted values | 1.3 µs; 0.38 µs per call with calls running on many components at once |

`go test -bench BenchmarkLive ./goscript/` reproduces the call numbers. `go test -v -run TestLiveMemory ./goscript/` reproduces the memory numbers.

## How it works

1. **Code generation:** `tern gen` finds each `.svelte` file with `<script lang="go" live>`.
   - It writes the script as a Go package under `.tern/goscript/live/`, with `//line` directives so errors point at the `.svelte` file.
   - It registers the package in `tern.gen.go`.
   - It writes TypeScript types for it under `.tern/goscript/types/`.
2. **Bindings:** a Svelte preprocessor replaces the Go script with TypeScript:
   - a `useLive()` call
   - a `$derived` per value
   - a function object per Func
   - `live`
3. **Server render:** the server renderer calls into Go to run `Setup` for each live component, and embeds the state in the page.
4. **In the browser:**
   - The component connects to `/_tern/live` and sends `mount` with its props.
   - The server runs `Setup` on a new goroutine and answers with state.
   - Calls, bound values and prop changes go up.
   - State, patches, results and events come down.

See [ADR 0010](adr/0010-live-components.md) for the design decisions.
