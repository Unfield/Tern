# Benchmarks

Numbers from `go test -bench` on an Apple M4 (10 cores), Go 1.27. All benchmarks run requests in parallel across every core and report time per request. They measure Tern itself; your loads and database queries come on top.

| Benchmark | Time | What it measures |
|---|---|---|
| `routes.BenchmarkMatch` | 2.3 µs | Matching a URL against 200 routes of mixed kinds |
| `kit.BenchmarkDataRequest` | 2.1 µs | A client-side navigation: two layout loads and a page load, encoded as JSON |
| `kit.BenchmarkDocument` | 3.2 µs | A full page load without server rendering |
| `kit.BenchmarkDocumentSSR` | 9.6 µs | A full page load with a small server render |
| `ssr.BenchmarkRenderSmall` | 5.9 µs | The renderer pool's own overhead |
| `ssr.BenchmarkRenderDashboard` | 0.5 ms | Server-rendering the dashboard example's login page, built from shadcn-svelte components |

Server rendering dominates, at roughly 2,000 renders per second for a real component tree on this machine. Pages a user navigates to on the client skip it entirely, and prerendered pages are served as static files.

## Running them

```sh
go test -run '^$' -bench . ./internal/routes/ ./kit/ ./internal/ssr/
```

`BenchmarkRenderDashboard` needs the dashboard example's server bundle; build it first with `go tool tern build` in `examples/dashboard`.

## Tuning

- `kit.WithSSRWorkers(n)` sets the number of rendering VMs (default: one per CPU). More workers than cores does not help.
- Pages that are the same for everyone can be prerendered (`const Prerender = true`).
- Pages that need no server-rendered HTML, such as dashboards behind a login, can opt out with `const SSR = false`.
