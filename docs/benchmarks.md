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

| | Tern, native | Tern, QuickJS | SvelteKit, 1 process | SvelteKit, 10 processes |
|---|---|---|---|---|
| A cacheable page | 32,120 to 87,584 | 25,665 to 60,252 | 2,611 to 17,459 | 5,464 to 19,458 |
| Client-side navigation | 43,549 | 37,640 | 3,478 | 8,031 |
| A page rendered for every request, small | 46,055 | 9,586 | 17,146 | 22,571 |
| A page rendered for every request, a 100-row table | 7,844 | 640 | 2,525 | 5,451 |
| Memory | about 240 MB | about 200 MB | about 500 MB | about 3 GB |

Since v1.5, `tern build` compiles the server renderer to Go ([ADR 0011](adr/0011-native-rendering.md)), and Tern is faster than SvelteKit on ten Node processes in every scenario. Pages that use what the compiler leaves to the JavaScript engine render with QuickJS, at the speeds in the QuickJS column.

## Running them

```sh
go test -run '^$' -bench . ./internal/routes/ ./kit/ ./internal/ssr/
```

`BenchmarkRenderDashboard` needs the dashboard example's server bundle; build it first with `go tool tern build` in `examples/dashboard`.

## Tuning

- `kit.WithSSRWorkers(n)` sets how many pages render at once. The default is one per CPU. Native rendering scales with every core; QuickJS VMs share a lock in the C runtime, so the engine uses at most 4 by default ([ADR 0007](adr/0007-ssr-throughput-and-memory.md)).
- `kit.WithSSRCache(bytes)` sets the size of the render cache (default 32 MiB, 0 to turn it off). A render depends only on the page's data and URL, so identical pages render once.
- Pages that are the same for everyone can be prerendered (`const Prerender = true`).
- Pages that need no server-rendered HTML, such as dashboards behind a login, can opt out with `const SSR = false`.
