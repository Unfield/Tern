# 0029. A build output for hosting platforms

- Status: Accepted
- Date: 2026-11-28

## Context

`tern build` makes one binary with the client, the prerendered pages and the server renderer inside. Running that binary is all a server needs (ADR 0016).

Hosting platforms that start apps on demand work differently. They serve static files from object storage or a CDN without starting the app, start it for everything else, and stop it when it's idle. A platform like that needs to know, without running the app:

- which URLs are static files, and the exact headers the app would send with each;
- where the server binary is, and how to start it and check that it's ready;
- when the app next has work to do while no request comes in, such as a scheduled job.

While checking what the app sends, this turned up a gap that doesn't depend on any platform: **prerendered pages are served without a Content-Security-Policy.** A prerendered page is saved during the build, with the nonce of that one prerender request still on its scripts, and then served as a static file. Static files get the security headers but no CSP. tern.land's home page is served like that today.

## Decision

### `tern build -out <dir>`

`tern build -out <dir>` writes a directory instead of a single binary:

```
<dir>/
  tern-output.json
  server/app          the binary, for GOOS/GOARCH as set (static, CGO off)
  static/             files under their URL paths, base path included
    index.html
    index.html.br
    index.html.gz
    docs/index.html
    docs/__data.json
    _app/immutable/client-CkA_0_f_.js
    ...
```

The binary still holds everything, so a platform that sends every request to it works too. `static/` is a copy for serving without the app. For the same source and Tern version, the output is the same byte for byte: no timestamps, `-trimpath`, and files in a fixed order.

`.br` files are new; today only `.gz` is written. A compressed file is kept only if it's smaller than the original.

### `tern-output.json`, version 1

```json
{
  "version": 1,
  "framework": { "name": "tern", "version": "1.20.0" },
  "server": {
    "path": "server/app",
    "goos": "linux",
    "goarch": "amd64",
    "env": { "port": "PORT", "host": "HOST" },
    "ready": "/readyz",
    "shutdown": { "signal": "SIGTERM", "grace_seconds": 10 }
  },
  "base_path": "",
  "immutable": ["/_app/immutable/"],
  "not_found": null,
  "headers": {
    "Cross-Origin-Opener-Policy": "same-origin",
    "Permissions-Policy": "camera=(), microphone=(), geolocation=()",
    "Referrer-Policy": "strict-origin-when-cross-origin",
    "X-Content-Type-Options": "nosniff",
    "X-Frame-Options": "DENY",
    "X-Permitted-Cross-Domain-Policies": "none"
  },
  "files": {
    "/": {
      "file": "index.html",
      "kind": "page",
      "encodings": ["br", "gzip"],
      "headers": {
        "Content-Type": "text/html; charset=utf-8",
        "Cache-Control": "public, max-age=0, must-revalidate",
        "Etag": "\"d2d483f4e8c8530b1ae3\"",
        "Content-Security-Policy": "default-src 'self'; script-src 'self' 'unsafe-hashes' 'sha256-7dQw…' 'sha256-…'; …"
      }
    },
    "/_app/immutable/client-CkA_0_f_.js": {
      "file": "_app/immutable/client-CkA_0_f_.js",
      "kind": "asset",
      "encodings": ["br", "gzip"],
      "headers": {
        "Content-Type": "text/javascript; charset=utf-8",
        "Cache-Control": "public, max-age=31536000, immutable"
      }
    }
  },
  "jobs": { "next": true }
}
```

How a platform serves it:

- **Matching.** A `GET` or `HEAD` whose path is a key in `files` is that file. Anything else, every other method included, goes to the server. There are no patterns, rewrites or trailing-slash rules: the keys are the URLs.
- **Kinds.** `page` is a prerendered page, `data` its `__data.json`, `endpoint` a prerendered endpoint such as `sitemap.xml`, `asset` a client or static file. A `page` matches only without a query string; with one it goes to the server, as the binary does it today. The others match with or without one.
- **Headers.** Send `headers`, then the file's own `headers` over them. Names are in Go's canonical form, such as `Etag`; compare them without regard to case. These are what the app sends for the file, captured from the app itself during the build, so `plugins/secure` and the app's options are in them. Headers that differ per request, such as `Date` and `X-Request-Id`, are left out.
- **Encodings.** `encodings` lists the compressed siblings, best first. Send `<file>.br` or `<file>.gz` with `Content-Encoding` when the request accepts it, and add `Vary: Accept-Encoding` whenever `encodings` isn't empty.
- **Immutable.** Paths under an `immutable` prefix never change content. A missing one is a 404 and never goes to the server.
- **Not found.** `not_found` is a key in `files` to send, with status 404, for such a miss, or null for a plain 404. Tern renders its error pages when they're needed, so for now it is null; a later version can fill it without a new `version`.
- **Versions.** New fields may be added in version 1, so readers ignore fields they don't know. A change that would mislead an old reader raises `version`.

`jobs` is present when the app has a `jobs.Queue`. `server.ready` is null without `plugins/health`.

### A CSP for prerendered pages

Prerendered pages get a Content-Security-Policy with hashes instead of a nonce. When the build saves a page, it removes the nonce attributes and hashes each inline script that runs. JSON data scripts don't run and aren't hashed. The page is then served with the app's policy, minus the nonce, plus those hashes. That is what the binary sends from now on, and the same header goes into `tern-output.json`. The `Report-Only` choice and `WithCSPSources` carry over. With `WithoutContentSecurityPolicy` the pages get no CSP, as now.

### Waking up for jobs

The queue already keeps the next run of each schedule (`tern_schedules.next_run`) and of each waiting job or retry (`run_at`) in the database. When it starts, it runs what is due, and a schedule missed while no instance was up runs once.

`jobs.Queue` gets

```go
// Next reports when a job or a scheduled run is next due (zero if none)
// and how many jobs are running now.
func (q *Queue) Next(ctx context.Context) (at time.Time, running int, err error)
```

A platform asks before it stops an idle app, and doesn't stop it while `running` is above zero. It starts the app again at `at`. Tern doesn't serve this on a route of its own: the platform's plugin serves it, on a path and with authentication of its choice. `/__tern/jobs/next` answering `{"next_run_at": "<RFC 3339>" | null, "running": <n>}` is the suggested shape. `tern-output.json` lists no crons, since the queue's own records are the schedule.

### Rate limits across instances

`plugins/ratelimit` and kit's default limit count in the process's memory. An app with several instances, or one that is stopped and started, then counts per instance and from zero. Both take a `cache.Store`, so Redis or a platform's key-value store holds the counts. Counting in a store uses fixed windows rather than a token bucket, since a store only has to count. That takes counters that expire, so stores can implement `cache.TTLCounter` (`IncrTTL`), as Memory, Disk and Redis do; in other stores the window's counters stay.

### Platform plugins live with the platform

Reading a platform's environment variables (database, Redis, object storage, mail) and wiring Tern to them is the platform's code, in the platform's repository. Everything it uses is public Tern API. Where a plugin needs something Tern doesn't offer, Tern adds it in a general form, as with `Queue.Next` and the store for rate limits.

## Consequences

- Hosting platforms can serve Tern's static files without starting the app, and with the same headers, from one documented file.
- Prerendered pages get a Content-Security-Policy, which they lacked. Their inline scripts are allowed by hash, so a page changed after the build loses its scripts, as it should.
- `tern build` without `-out` is unchanged.
- `tern-output.json` is a format to keep stable: fields are only added in version 1.
- Rate limits can hold across instances. Fixed windows allow up to twice the rate at a window's edge, which is acceptable for abuse protection. The in-memory token bucket stays the default.
