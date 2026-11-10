# 0007. Server-rendering throughput and memory

- Status: Accepted
- Date: 2026-11-10

## Context

A load test of the dashboard example found two problems with server rendering in production:

1. **Throughput fell with more workers.** The QuickJS VMs run on modernc.org's C-to-Go runtime, whose allocator takes one global lock. QuickJS allocates on every JavaScript function call, so the VMs contend for that lock. On a 10-core machine, renders per second peaked at 4 workers (about 3,150/s) and halved at 10, the old default.
2. **Memory grew without limit,** by about 5 KB per render. QuickJS never frees a `WeakMap` entry whose value refers to its key. core-js's `URL` polyfill stores state exactly that way, and every page keeps its `URL` in `page.url`, so every render leaked one.

## Decision

- **Fewer default workers.** `ssr.NewPool` defaults to `min(GOMAXPROCS, 4)` workers. `kit.WithSSRWorkers` still overrides it.
- **Our own `URL` polyfill.** `URL` and `URLSearchParams` are implemented in Tern (`packages/tern/src/server/url.ts`) with private fields instead of a `WeakMap`, and tested against Node's implementation. core-js is no longer a dependency.
- **VM retirement.** Each VM is retired after 10,000 renders and replaced in the background (`ssr.Options.MaxRenders`). This bounds memory that third-party libraries hold across renders, which Tern cannot fix for them.
- **A render cache.** The output of a render depends only on the payload and the URL; hydration already requires that. So production keeps up to 32 MiB of rendered HTML keyed by both, and coalesces concurrent renders of the same page (`kit.WithSSRCache`). Pages that look the same for many visitors render once.

## Consequences

- On the reference machine:
  - Unique renders went from 2,200 to 3,300 requests per second, with p99 latency down from 21 ms to 13 ms.
  - Repeated pages are served from the cache at about 46,000 requests per second.
  - Memory levels off instead of growing.
- A component whose server output depends on the clock or on randomness can be served from the cache. Such components already render differently in the browser and do not hydrate cleanly, so the guide says to keep render output a function of the data. `kit.WithSSRCache(0)` turns the cache off.
- The allocator lock remains the ceiling for uncached renders. Lifting it would mean patching the C runtime or running several processes. We will revisit if real apps hit the limit.
