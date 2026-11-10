# 0011. Native rendering

- Status: Proposed
- Date: 2026-11-10

## Context

[benchmarks/vs-sveltekit](../../benchmarks/vs-sveltekit) shows Tern renders 1.8× to 8.5× slower than SvelteKit on Node when a page can't come from the cache. Profiling a render of its 100-row table (2.9 ms) found:
- **Tern's own cost is small:** about 0.15 ms in Go and 0.25 ms parsing the payload.
- **The rest is Svelte's render code running in QuickJS:**
  - getters for props and derived values take half
  - each escaped value runs a regular expression, about 1 µs
  - string concatenation and the C runtime's allocator take most of what's left
- **VMs stop scaling at four,** as [ADR 0007](0007-ssr-throughput-and-memory.md) found.

Faster JavaScript engines (V8, JavaScriptCore) would need cgo and give up the single static binary. They would reach Node's speed, not beat it.

[benchmarks/native-spike](../../benchmarks/native-spike) wrote by hand the Go a compiler would generate for that page. It renders the same HTML as Svelte, byte for byte, in 22 µs: 130× faster than QuickJS, 18× faster than V8, and it scales with every core.

## Decision (proposed)

Compile the server build's components to Go at build time, render with them, and fall back to the JavaScript engine for anything that can't be compiled.

### What is compiled, and from what

- **Input:** each module of the server build, as Vite hands it to the bundler: Svelte's server output for components (TypeScript already stripped), and the JavaScript of local modules and libraries.
  - A Vite plugin captures each module's code, its ESTree from acorn, and its resolved imports, when `tern build` asks (`.tern/native/ir.json`).
  - Svelte decides the markup and hydration markers. The compiler only has to keep the meaning of the JavaScript.
- **Svelte's server runtime** (`svelte/internal/server`: the `Renderer`, `escape`, `attr`, `attributes`, `clsx`, `props_id`, …) is written in Go once per supported Svelte version, from Svelte's source, and pinned: `tern build` compiles natively only with a Svelte version it supports.
- **Tern's runtime** that components import (`page`, `t`, …) is provided natively from what Go already knows. The route, its layouts and the data merging are Go's own.

### How JavaScript becomes Go

- **Exact values first.**
  - **The value model:** compiled code works on a Go model of JavaScript values: `undefined`, `null`, booleans, numbers, strings with UTF-16 semantics, objects in property order with getters, arrays, functions, symbols. Operators and built-ins follow the spec, and are tested against Node.
  - **The data:** loads' data comes from the same JSON payload the browser gets, so `omitempty`, `MarshalJSON` and integers beyond 2^53 can't differ.
- **Types as an optimisation.**
  - **Where:** when a value's type is known, typed Go is emitted instead of dynamic operations. Types come from the loads' Go structs (through `encoding/json`'s rules), literals and inference.
  - **Proof:** every typed path must produce the same result as the dynamic one, proven by the same differential tests.
- **Helpers on the hot path,** such as `escape` and `attr`, are native Go.

### When it can't be compiled

- **At build time:** a construct the compiler doesn't support (a `Proxy`, a generator, an async `{#await}`) marks its module unsupported, and with it every route whose components use it. Those routes render with QuickJS, as now. `tern build` reports which routes are native and, for the rest, the first reason.
- **At run time:** compiled code that meets a case it can't handle stops, and that request renders with QuickJS.

### Proving it

- **Differential tests:** each example's routes render both ways and must produce identical HTML.
- **Shadow mode:** `TERN_NATIVE_VERIFY=1` renders every page both ways in a running app, serves QuickJS's HTML, and logs any difference. The end-to-end suites run with it.
- **Value tests:** each operator and built-in has cases checked against Node.

### Phases

1. **The pipeline:** capturing modules, the dynamic compiler, Svelte's runtime in Go, kit's native path with fallback and shadow mode, the differential tests, and the benchmark app native.
2. **Types:** typed fast paths from Go structs, to approach the spike's speed.
3. **Coverage:** the examples and Tern UI native, driven by what `tern build` reports. Bindings (`copy`/`subsume`), `<select>` options, `{#await}`, and libraries such as `clsx` and `tailwind-merge`.

## Consequences

- **Rendering speed:** native pages render about 100× faster than today, with no per-render JavaScript and no lock. The render cache matters less.
- **Fallback:** components that can't be compiled work as before.
- **New risks:**
  - **Svelte's internal server output can change between versions,** so every Svelte upgrade needs the differential tests to pass first.
  - **JavaScript semantics in the supported subset must be exact.** Numbers, string slicing by UTF-16 code units, and truthiness each need tests against Node.
- **Scope:** this is the largest piece of v1.6, before the v1.5 work.
