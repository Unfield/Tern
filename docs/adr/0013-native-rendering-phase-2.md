# 0013. Native rendering, phase 2

- Status: Accepted
- Date: 2026-11-11

## Context

[ADR 0011](0011-native-rendering.md) planned phase 2 as typed fast paths: use the loads' Go structs to compile components that read page data without JavaScript property lookups, to approach the hand-written spike's 22 µs for the benchmark's 100-row table.

Before building that, we profiled an uncached request for that page (`benchmarks/vs-sveltekit/tern`, `BenchmarkItems`), which took 352 µs at v1.7.0. The time wasn't where the plan assumed:

- **Property reads were already cheap.** Inline caches make `item.price` a shape check and a slot load, a few nanoseconds. Typing them would save little.
- **The data path cost more than the reads:** parsing the payload's JSON in the realm (about 50 µs), and five full copies of the page while the document was put together.
- **Svelte's own machinery:** `Renderer.push` and `#collect_content`, which joins the page's pieces one `+=` at a time.
- **Svelte's per-element helpers,** compiled from JavaScript: about 800 ns per table row in `attr`, `attr_class`, `stringify`, `ensure_array_like` and `escape_html`.
- **Allocation and garbage collection,** about a fifth of the time.

## Decision

Phase 2 makes the hot path native where the profile pointed:

- **Page data as Go values.** kit hands native rendering the loads' results along with their JSON. `internal/jsvalue` makes the JavaScript values `JSON.parse` would make of `internal/jsonenc`'s JSON, value for value and in the same key order, without writing or parsing it. A differential test and a fuzz test hold the two to each other. The JavaScript engine still reads the JSON.
- **Method intrinsics.** Intrinsics ([ADR 0011](0011-native-rendering.md)) extend to class methods. They can use the class's private names and constructor, and the compiled method for the cases their Go code leaves to it. `Renderer.push` appends to `#out` directly. `#collect_content` walks the output tree once and joins each part into one string of the right size.
- **More helper intrinsics.** `attr`, `to_class`, `attr_class`, `stringify` and `ensure_array_like` are written in Go. Each takes its fast path only when it computes what the JavaScript computes without running other code. Otherwise it calls the compiled function: for objects whose conversion runs code, getters, non-ASCII class names, or module constants other than the expected ones. Their templates can read module bindings (`{@name}`) and the compiled function (`{orig}`).
- **A conformance case for the intrinsics.** Copies of Svelte's helper sources sit at paths that trigger the intrinsics, and their results across edge cases are checked against Node. It found a bug in the runtime's String objects, now fixed.
- **One buffer for the document.** The payload and the page are written into pooled buffers, from their pieces, instead of being copied at each step.

Typed fast paths stay unbuilt: the profile shows they would save little while inline caches do their job.

## Consequences

- The benchmark's uncached `/items` takes 272 µs, from 352 µs at v1.7.0 and 425 µs at v1.6.0. It allocates 352 KB, from 573 KB. Every example renders the engine's HTML (`TERN_NATIVE_VERIFY=1`).
- What remains is spread thinly: the compiled components' own work, about 10 ns per JavaScript call, and allocation, much of it strings boxed as `Value`. Going much further means a different value representation (unboxed numbers and strings) or a rewrite of the call protocol. Either is larger than this phase, and would be its own decision.
- A load's result must not change after the load returns: native rendering reads it after it was encoded. That has to hold for deferred values and streaming already.
- Intrinsics stay tied to Svelte's source by fingerprint. A Svelte release that changes one of these functions gets it compiled from its source again, slower but correct, and `tern build` reports it.
