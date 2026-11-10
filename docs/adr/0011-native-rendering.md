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

Compile components to Go at build time, and render with them where they can be compiled, falling back to the JavaScript engine for the rest.

- **What is translated:** Svelte's own server output, not the component source.
  - Svelte decides the markup and the hydration markers.
  - The translator only turns its JavaScript into Go: `push`es of template strings, a small set of helpers (`escape`, `attr`, `attr_class`, `attr_style`, `stringify`, `ensure_array_like`, …), loops, conditionals, snippets and derived values.
- **Types come from Go:** a load's data is a Go struct, so props have Go types. Expressions such as `item.price.toFixed(2)` or `items.filter((i) => i.stock === 0).length` translate to typed Go, with JavaScript's semantics for numbers and strings in a small runtime.
- **A subset, with a fallback:**
  - **Supported:** property access, arithmetic and comparison, template strings, conditionals, array methods with arrow functions, and known string and number methods.
  - **Everything else:** a component that uses anything else, such as an imported library, stays in JavaScript.
  - **Mixing:** both can mix in one page, component by component.
  - **Reporting:** `tern build` reports which components are native and why the others aren't.
- **Where it runs:** the translator is TypeScript in the Vite plugin, with Svelte's own parser, since builds already run Node. It writes Go to `.tern/native/`, which `tern gen` wires in like routes.
- **Proof:** a differential test renders every component of the examples both ways and requires identical HTML. Svelte's supported versions are pinned and tested, since its server output is internal.

## Consequences

- **Rendering speed:** native pages render about 100× faster than today, with no per-render JavaScript and no lock. The render cache matters less.
- **Fallback:** components that can't be compiled work as before.
- **New risks:**
  - **Svelte's internal server output can change between versions,** so every Svelte upgrade needs the differential tests to pass first.
  - **JavaScript semantics in the supported subset must be exact.** Numbers, string slicing by UTF-16 code units, and truthiness each need tests against Node.
- **Scope:** this is the largest piece of v1.6, before the v1.5 work.
