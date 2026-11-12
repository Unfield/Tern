# 0022. Keep boxed values in native rendering

- Status: Accepted
- Date: 2026-11-12

## Context

[ADR 0013](0013-native-rendering-phase-2.md) left one item for phase 3: a value representation that doesn't box numbers and strings. Today `js.Value` is `any`, so turning a Go string or float into a value allocates, and the idea was a tagged struct holding numbers and string headers inline. That would touch the whole runtime in `native/js` (about 11,000 lines) and every line the compiler generates (the status example's generated code uses `js.Value` about 58,000 times).

Before deciding, we profiled the render benchmark (`benchmarks/vs-sveltekit/tern`, one core). The `/items` page takes 277 µs, all of it rendered natively:

- **Allocating costs, collecting doesn't.** With `GOGC=off` the page is as fast as with the collector on. `mallocgc` is about 22% of CPU, and growing the heap (`madvise` from `sysUsedOS`) another 8%.
- **58% of allocations are 16 bytes:** boxed strings, boxed numbers (packed in 16-byte blocks by Go's tiny allocator) and short strings. They're 13% of the bytes allocated.
- **Property reads through inline caches** are about 17% of CPU, and **turning the page data into JS values** about 9%.
- **The biggest single source of allocations** is rendering HTML: each chunk is concatenated, boxed and passed to `renderer.push`, which appends it to the renderer's `#out` array. About twenty other places in Svelte's renderer read, copy and compare that array, so it has to stay a JS array of values.

## Decision

Keep `js.Value` as `any`.

- **The gain is capped and small.** If every 16-byte allocation went away, and an allocation's cost scales with its count, that's about half of the 22% spent allocating: roughly 10–15% on this page. A tagged struct takes 24–32 bytes where an interface takes 16, so arrays, arguments and object slots get bigger, and copying them costs more. That would give back part of the gain.
- **The cost is a rewrite** of the runtime and the code generator, with a long tail of semantic bugs in a part of Tern whose job is to behave exactly like the engine.
- **Smaller changes target the same costs:** fewer allocations on the paths that make them (concatenating and pushing HTML, the arguments slices of calls, converting page data), each measured on its own with `benchmarks/compare.sh`.

The roadmap item is closed as decided against. `fn.toString()` matching the bundle moves to later; it only changes what verify mode compares.

## Consequences

- Native rendering stays where it is: `/items` in 277 µs and the home page in 55 µs, in one process, with no JavaScript engine needed.
- The allocation work above is done one measured change at a time, and stays compatible with the runtime as it is.
- If a profile of a real app ever shows boxing as the main cost, this can be revisited with that profile; this one doesn't.

## Since

- **Boxes from slabs** (v1.15): strings and numbers that become Values on the hot paths (template literals, `+`, page data, `String()`, `toFixed`, `slice` and Svelte's `attr`) take their boxes from slabs of 128 in the realm, one allocation for many, and `class:` directives are read without copying their entries. On `/items`, allocations went from 5,501 to 3,285 a render and the time from 268 to 254 µs. The values are still `any`: nothing else changed.
- **Still open:** the arguments slices of calls, which a callee can keep (`arguments`, rest parameters, `bind`), so reusing them needs every path that keeps them to copy first.
