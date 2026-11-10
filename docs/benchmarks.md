# Benchmarks

Numbers from `go test -bench` on an Apple M4 (10 cores), Go 1.27. All benchmarks run requests in parallel across every core and report time per request. They measure Tern itself; your loads and database queries come on top.

| Benchmark | Time | What it measures |
|---|---|---|
| `routes.BenchmarkMatch` | 2.3 µs | Matching a URL against 200 routes of mixed kinds |
| `kit.BenchmarkDataRequest` | 2.1 µs | A client-side navigation: two layout loads and a page load, encoded as JSON |
| `kit.BenchmarkDocument` | 3.2 µs | A full page load without server rendering |
| `kit.BenchmarkDocumentSSR` | 9.6 µs | A full page load with a small server render |
| `ssr.BenchmarkRenderSmall` | 5.9 µs | The renderer pool's own overhead |
| `ssr.BenchmarkRenderDashboard` | 0.3 ms | Server-rendering the dashboard example's login page, built from shadcn-svelte components |

Server rendering dominates, at roughly 3,300 renders per second for a real component tree on this machine. Pages a user navigates to on the client skip it entirely, prerendered pages are served as static files, and repeated renders come from the render cache.

## Load test

The dashboard example built with `tern build`, 32 concurrent connections, on the same machine:

| Scenario | Requests/s | p50 | p99 |
|---|---|---|---|
| The login page, served from the render cache | 46,000 | 0.6 ms | 2.4 ms |
| A unique URL per request, so every request renders | 3,300 | 9.4 ms | 12.6 ms |

Memory levels off at about 240 MB with the render cache full, and stays flat over hundreds of thousands of renders.

## Compared with SvelteKit

[benchmarks/vs-sveltekit](../benchmarks/vs-sveltekit) serves the same Svelte components with Tern and with SvelteKit on Node. Summary, in requests per second:

| | Tern | SvelteKit, 1 process | SvelteKit, 10 processes |
|---|---|---|---|
| A cacheable page | 25,665 to 60,252 | 2,611 to 17,459 | 5,464 to 19,458 |
| Client-side navigation | 37,640 | 3,478 | 8,031 |
| A page rendered for every request, small | 9,586 | 17,146 | 22,571 |
| A page rendered for every request, a 100-row table | 640 | 2,525 | 5,451 |
| Memory | about 200 MB | about 500 MB | about 3 GB |

Tern is faster wherever its Go side or the render cache answers. It is slower for every request that must be rendered, by 1.8× to 8.5×, because QuickJS interprets JavaScript and its VMs stop scaling at four. Speeding that up is the plan for v1.6.

## Running them

```sh
go test -run '^$' -bench . ./internal/routes/ ./kit/ ./internal/ssr/
```

`BenchmarkRenderDashboard` needs the dashboard example's server bundle; build it first with `go tool tern build` in `examples/dashboard`.

## Tuning

- `kit.WithSSRWorkers(n)` sets the number of rendering VMs. The default is one per CPU, at most 4: the VMs share a lock in the C runtime, and on a 10-core machine throughput peaks at 4 workers and halves at 10 ([ADR 0007](adr/0007-ssr-throughput-and-memory.md)).
- `kit.WithSSRCache(bytes)` sets the size of the render cache (default 32 MiB, 0 to turn it off). A render depends only on the page's data and URL, so identical pages render once.
- Pages that are the same for everyone can be prerendered (`const Prerender = true`).
- Pages that need no server-rendered HTML, such as dashboards behind a login, can opt out with `const SSR = false`.
