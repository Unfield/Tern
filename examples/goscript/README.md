# goscript

Tern's experimental `<script lang="go">`: Svelte components whose logic is Go, compiled to WebAssembly, with runes (`goscript.State`, `goscript.Derived`, `goscript.Effect`).

```sh
npm install
go tool tern dev
```

`src/lib/LiveRoom.svelte` is a live component: its Go runs on the server, shares a click counter and an online count between every open page, and renders on the server with its props.

`src/lib/Counter.svelte` shows state, derived values, effects, props and events. `src/lib/Primes.svelte` and `src/lib/PrimesJS.svelte` run the same prime sieve in Go and in TypeScript, so you can compare them.

See [docs/experimental-go-script.md](../../docs/experimental-go-script.md) for how it works, what it costs and where it could go.
