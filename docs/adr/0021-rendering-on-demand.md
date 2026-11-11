# 0021. Rendering on demand and caching

- Status: Accepted
- Date: 2026-11-12

## Context

Prerendering makes pages as fast as static files, but only for what's known at build time, and a change needs a build. Rendering every request is always current but repeats the same work for every visitor. Most content sites and the public pages of apps are in between: the same for everyone, changing a few times a day.

Caching a rendered page is easy to get dangerously wrong. A page rendered for one signed-in user, cached and served to the next, leaks the first user's data. Caches in each process also disagree after a change, unless invalidation reaches all of them.

## Decision

- **A route opts in with `const Revalidate`,** seconds, a `time.Duration`, or `-1` for "until revalidated". Its pages are rendered on the first request, served from the cache after, and rendered again in the background once stale, with the old copy served meanwhile.
- **Cached pages are rendered as anyone sees them:** without the request's cookies and `Authorization` header, with `Set-Cookie` dropped. That makes a leak impossible by construction, instead of relying on each load to know it's being cached. Pages for signed-in users stay uncached.
- **Tags name the data a page used** (`c.Tag`), and `Revalidate(tags...)` makes everything with them stale. Tags have versions in the store, and an entry records the versions it was made with, so an invalidation reaches every process sharing the store without messages between them.
- **One cache, on a store:** memory (the default, per process), disk, or Redis through a RESP client of Tern's own, with no dependency. Load results use the same cache through `cache.Get`, with one call per key at a time.
- **Each served copy gets a fresh CSP nonce,** in place of a placeholder the page was rendered with.
- **`const CacheControl`** sets the header for CDNs, and **`const CSR = false`** sends a page with no JavaScript. Its data URL answers 204, which the client router already takes as "load this page in full".

## Consequences

- Public pages are served from memory or Redis with no loads run, and change within seconds of their data.
- A load that reads the user on a cached page sees nobody. That's the point, but it can surprise; the docs say so first.
- Each query string is its own copy, so pages that take arbitrary queries can fill the cache. The memory store evicts the least recently used; Redis should have a `maxmemory` policy.
- Tags found while rendering are recorded after the render, so an invalidation during a page's render of a tag only that render found can be missed until the page's next revalidation. Tags given up front aren't affected.
