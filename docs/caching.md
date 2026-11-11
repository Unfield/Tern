# Caching and rendering on demand

Some pages are the same for everyone and change now and then: a blog post, a product page, a docs page, a changelog. Prerendering makes them at build time; rendering on demand makes them on the first request and keeps them, so they're as fast as prerendered pages but change without a deploy.

## Pages rendered on demand

```go
// src/routes/blog/slug.param/page.server.go
package post

const Revalidate = 300 // seconds; or 5 * time.Minute

func Load(c *kit.Ctx) (Post, error) {
	c.Tag("posts", "post:"+c.Param("slug"))
	return db.Post(c, c.Param("slug"))
}
```

The page is rendered on its first request and served from the cache after. Once it's older than `Revalidate`, the next request still gets the cached copy, at once, while a new one is rendered in the background (stale-while-revalidate). `const Revalidate = -1` (`kit.Forever`) keeps it until a tag it used is revalidated.

**It's the page as anyone sees it.** The cached copy is rendered without the request's cookies and `Authorization` header, so nothing about the visitor can end up in it: `accounts.Current` is nil in its loads, and cookies they set are dropped. Hooks don't run for the copies served. Each language gets its own copy, and so does each query string. Pages for signed-in users stay uncached; put what they show apart from the public page, or load it in the browser.

Each response says what happened in `X-Tern-Cache`: `MISS` (rendered now), `HIT` or `STALE`. Pages that fail, such as a 404, are served but not kept. Under `tern dev`, nothing is cached.

## Revalidating

`c.Tag(...)` in a load names the data the page used. After the data changes, revalidate the tags, and every page and cached load result that used them is rendered again on its next request:

```go
"publish": kit.FormAction(func(c *kit.Ctx, in Post) (Done, error) {
	...
	return Done{}, c.Revalidate("posts", "post:"+in.Slug)
}),
```

Outside a request, such as in a job, `app.Revalidate(ctx, tags...)` does the same. With a shared cache, revalidating in one process refreshes the pages in all of them.

## The cache

Pages, and loads that keep results, use the App's cache: 64 MB in memory per process by default. For several processes or machines, give them one they share:

```go
store, err := redis.New(os.Getenv("REDIS_URL")) // github.com/Unfield/Tern/cache/redis
app, err := ternApp(kit.WithCache(cache.New(store)))
```

| Store | |
|---|---|
| `cache.Memory(maxBytes)` | in the process, least recently used first out. Each process has its own |
| `cache.Disk(dir)` | a file per entry; processes on one machine, or on a shared volume, share it |
| `redis.New(url)` | Redis, Valkey or anything that speaks its protocol, shared by every process. `redis://:password@host:6379/0`, or `rediss://` for TLS |

A `cache.Store` of your own works too: it gets, sets with a TTL, deletes and increments.

## Caching load results

```go
func Load(c *kit.Ctx) (Page, error) {
	stats, err := cache.Get(c, c.Cache(), "stats:"+team, cache.For(time.Minute, "stats"), func(ctx context.Context) (Stats, error) {
		return db.Stats(ctx, team) // slow
	})
	...
}
```

`cache.Get` returns the cached value, decoded from JSON, or calls the function, keeps its result and returns it. While it runs, other requests for the same key in the process wait for it instead of running it too. `cache.Options` has `TTL`, `Stale` (serve the old value for this long while a new one is made) and `Tags`; `c.Cache().Invalidate(ctx, tags...)` is what `Revalidate` calls. Unlike pages, cached load results can be per user: put the user in the key.

## Cache headers

`const CacheControl = "public, max-age=60, stale-while-revalidate=600"` sets the `Cache-Control` header of the route's successful responses, the page and its data, for a CDN or the browser. Without it, pages are `private, no-cache` and their data `private, no-store`. Use `public` only for pages that are the same for everyone.

## Pages without JavaScript

```go
const CSR = false
```

The page is sent as the server rendered it, with its styles and no scripts: nothing to download, parse or hydrate. It suits pages with nothing interactive, such as articles, legal pages and landing pages. Links to it from other pages load it in full. Forms on it work without JavaScript, as HTML forms do. Deferred values aren't streamed on it, so await what it needs in its load. It needs server rendering: `SSR = false` with `CSR = false` is an error.
