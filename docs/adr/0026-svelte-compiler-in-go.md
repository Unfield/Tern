# 0026. Svelte's compiler in Go

- Status: Proposed
- Date: 2026-11-12

## Context

`tern dev` and `tern build` need Node, for Vite and for Svelte's compiler, which is JavaScript. The app that's shipped doesn't: it runs, and since v1.13 mostly renders, without Node or a JavaScript engine. A build without Node would need Svelte's compiler without Node first, and Tern has a way to get it: native rendering compiles JavaScript to Go ([ADR 0011](0011-native-rendering.md)), and it could compile the compiler.

## The spike

We compiled `svelte/compiler` (Svelte 5.57) to Go with the native compiler and compiled every `.svelte` file in the repository with it, 445 components from Tern UI, the examples and tern.land, comparing the output with Node's byte for byte.

- **Correctness:** all 445 match, for server and for client output.
- **Speed:** 3.1 s for the 445, cold, against about 0.5 s of compiling in Node; about 3 ms a component warm, against about 1 ms. Making the realm takes 90 ms. Slower than V8's JIT, and fast enough for builds and for recompiling a file on save.
- **Size:** the compiled compiler is 6.8 MB of code, on a CLI of 26 MB.
- **The input:** the compiler and its dependencies were compiled as one bundled module. Recorded module by module, as the Vite plugin records an app, three of its dependencies are CommonJS (`aria-query`, `axobject-query` and a UMD build of `@jridgewell/resolve-uri`), which the native compiler doesn't support.

It found three bugs in native rendering, fixed with conformance cases, which apps could hit too:

- `var` declared in a loop or block at a module's top level crashed the compiler.
- `\p{ID_Start}` and `\p{ID_Continue}` in regular expressions failed to compile.
- A sticky or global regular expression after non-ASCII text started at the wrong place: `lastIndex` was given to the regexp engine as a rune index where it takes a byte offset.

## Proposal

A `tern build` that needs only Go, for apps without Vite plugins:

- **The compiler** is compiled to Go once per Svelte version, when Tern is released, and ships in the CLI.
- **Bundling** the client, TypeScript and CSS is esbuild's, which is Go.
- **npm packages** still come from `node_modules`, installed as today; resolving them is esbuild's.
- **Vite stays** the default, and the path for apps with Vite plugins, such as Tailwind's, and for `tern dev`'s hot reloading.

Before that: the build has to match Vite's output closely enough (chunks, the asset manifest, CSS order), which is the larger part of the work; and CommonJS modules need compiling, for the compiler's dependencies recorded as they are and for apps' own.

## Consequences

- If it's built, CI and Docker builds of most apps need Go only, and Tern controls its build's speed and versions.
- Two build paths to keep matching, with tests that build the examples both ways and compare.
- The fixes above help native rendering now, whether or not the build follows.
