# Tern vs SvelteKit

The same app twice: one served by Tern, one by SvelteKit on Node with `adapter-node`. Both render the same Svelte components from `shared/`, so the HTML is the same; only the server differs.

- **Pages:** `/` is a small page; `/items` is a 100-row table, about 35 KB of HTML. Both have a layout and a load.
- **Loads:** each app's `/items` load builds the same 100 items, Tern in Go and SvelteKit in JavaScript.
- **Versions:** Tern at this commit; SvelteKit 3.2.0, adapter-node 6.0.2, Svelte 5.57.3, Vite 8, Node 24.13.

## Results

Apple M4 (10 cores), 32 connections, 10 seconds after 3 seconds of warm-up. The load generator runs on the same machine. Requests per second, then median and 99th-percentile latency in ms, then resident memory at the end.

| Scenario | Tern, native (v1.5) | Tern, QuickJS (v1.4) | SvelteKit, 1 Node process | SvelteKit, 10 Node processes |
|---|---|---|---|---|
| `/`, the same URL each time | **87,584** · 0.3 / 1.1 · 36 MB | 60,252 · 0.4 / 1.8 · 21 MB | 17,459 · 1.6 / 4.8 · 60 MB | 19,458 · 0.8 / 15.2 · 636 MB |
| `/`, a new URL each time | **46,055** · 0.5 / 3.1 · 183 MB | 9,586 · 3.3 / 4.6 · 36 MB | 17,146 · 1.6 / 4.8 · 298 MB | 22,571 · 0.8 / 12.1 · 2.7 GB |
| `/items`, the same URL each time | **32,120** · 0.8 / 4.7 · 236 MB | 25,665 · 1.0 / 5.0 · 118 MB | 2,611 · 11.7 / 23.1 · 304 MB | 5,464 · 4.9 / 20.1 · 2.7 GB |
| `/items`, a new URL each time | **7,844** · 3.5 / 12.3 · 236 MB | 640 · 50.1 / 54.6 · 205 MB | 2,525 · 12.1 / 24.1 · 523 MB | 5,451 · 4.9 / 21.9 · 2.9 GB |
| `/items/__data.json` (client-side navigation) | **43,549** · 0.6 / 2.9 · 236 MB | 37,640 · 0.7 / 3.1 · 207 MB | 3,478 · 8.5 / 16.8 · 526 MB | 8,031 · 3.0 / 15.5 · 3.0 GB |

Native is `tern build` compiling the server renderer to Go ([ADR 0011](../../docs/adr/0011-native-rendering.md)); the native column is in `results-native-2026-11-10.jsonl`, the rest in `results-2026-11-10.jsonl`. The QuickJS column is the same app with `TERN_NATIVE=0`.

Uncached `/items` renders with different numbers of Tern's QuickJS VMs (`kit.WithSSRWorkers`), before native rendering:

| Workers | 1 | 2 | 4 (default) | 8 |
|---|---|---|---|---|
| Requests per second | 345 | 562 | 688 | 433 |

## What it shows

With native rendering, Tern is faster than SvelteKit on ten Node processes in every scenario, from one process and a tenth of the memory: 1.4× for an uncached 100-row table, 2× for an uncached small page. The rest of this section describes v1.4, with QuickJS, which is still what renders pages native rendering can't.

- **Where Tern wins:**
  - **Pages the render cache can answer**, the same for many visitors: 3.5× to 10× SvelteKit on one process, which has no render cache.
  - **Client-side navigation**: Go loads and JSON answer 10.8× faster than one Node process, and 4.7× faster than ten.
  - **Memory**: one Tern process stays around 200 MB. Ten Node processes, which it takes for SvelteKit to use the machine, need about 3 GB.
- **Where Tern loses: every request that must be rendered.**
  - **Small pages:** 1.8× slower than one Node process.
  - **Large pages:** 3.9× slower than one Node process and 8.5× slower than a cluster, with a p99 of 55 ms against 22 ms.
- **Why it loses:**
  - **Speed per render:** one Tern VM renders the table in about 2.9 ms; one Node process in about 0.4 ms. QuickJS interprets JavaScript, while V8 compiles it.
  - **Cores:** VMs stop scaling at 4, because the C-to-Go runtime's allocator has one lock ([ADR 0007](../../docs/adr/0007-ssr-throughput-and-memory.md)).
  - **Who it affects:** pages that are different for every user and render a lot of markup, such as signed-in dashboards with big tables.

## In CI

The [Benchmarks workflow](../../.github/workflows/bench.yml) runs on every push and pull request:

- **Render benchmarks:** [benchmarks/compare.sh](../compare.sh) builds this app at the commit before and at the new one, renders `/`, `/items` and `/items/__data.json` in-process (`render_bench_test.go`), alternating between the two on one machine, and compares them with benchstat. It fails when a page is significantly slower by more than 10%.
- **Tern vs SvelteKit:** on pushes to `main`, `run.sh` runs for 5 seconds per scenario and the table lands in the job summary, with `results.jsonl` kept as an artifact for 90 days. [summary.sh](summary.sh) compares Tern's numbers and binary size with the last successful run on `main` and warns when a scenario is more than 10% slower or the binary more than 5% larger. Shared machines are noisy: compare runs with each other, not with the numbers above.

Locally, `./benchmarks/compare.sh main` compares the working tree with `main`.

## Running it

```sh
(cd tern && npm install && npm run build)
(cd sveltekit && npm install && npm run build)
./run.sh   # writes results.jsonl
```

`DUR`, `WARM`, `CONNS` and `WORKERS` (SvelteKit's cluster size) change the defaults.
