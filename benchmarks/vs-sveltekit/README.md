# Tern vs SvelteKit

The same app twice: one served by Tern, one by SvelteKit on Node with `adapter-node`. Both render the same Svelte components from `shared/`, so the HTML is the same; only the server differs.

- **Pages:** `/` is a small page; `/items` is a 100-row table, about 35 KB of HTML. Both have a layout and a load.
- **Loads:** each app's `/items` load builds the same 100 items, Tern in Go and SvelteKit in JavaScript.
- **Versions:** Tern at this commit; SvelteKit 3.2.0, adapter-node 6.0.2, Svelte 5.57.3, Vite 8, Node 24.13.

## Results

Apple M4 (10 cores), 32 connections, 10 seconds after 3 seconds of warm-up. The load generator runs on the same machine. Requests per second, then median and 99th-percentile latency in ms, then resident memory at the end.

| Scenario | Tern | SvelteKit, 1 Node process | SvelteKit, 10 Node processes |
|---|---|---|---|
| `/`, the same URL each time | **60,252** · 0.4 / 1.8 · 21 MB | 17,459 · 1.6 / 4.8 · 60 MB | 19,458 · 0.8 / 15.2 · 636 MB |
| `/`, a new URL each time | 9,586 · 3.3 / 4.6 · 36 MB | 17,146 · 1.6 / 4.8 · 298 MB | **22,571** · 0.8 / 12.1 · 2.7 GB |
| `/items`, the same URL each time | **25,665** · 1.0 / 5.0 · 118 MB | 2,611 · 11.7 / 23.1 · 304 MB | 5,464 · 4.9 / 20.1 · 2.7 GB |
| `/items`, a new URL each time | 640 · 50.1 / 54.6 · 205 MB | 2,525 · 12.1 / 24.1 · 523 MB | **5,451** · 4.9 / 21.9 · 2.9 GB |
| `/items/__data.json` (client-side navigation) | **37,640** · 0.7 / 3.1 · 207 MB | 3,478 · 8.5 / 16.8 · 526 MB | 8,031 · 3.0 / 15.5 · 3.0 GB |

Uncached `/items` renders with different numbers of Tern's rendering VMs (`kit.WithSSRWorkers`):

| Workers | 1 | 2 | 4 (default) | 8 |
|---|---|---|---|---|
| Requests per second | 345 | 562 | 688 | 433 |

## What it shows

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

## Running it

```sh
(cd tern && npm install && npm run build)
(cd sveltekit && npm install && npm run build)
./run.sh   # writes results.jsonl
```

`DUR`, `WARM`, `CONNS` and `WORKERS` (SvelteKit's cluster size) change the defaults.
