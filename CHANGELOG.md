# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Before v1.0.0, minor versions may contain breaking changes.

## [Unreleased]

### Added

- [examples/status](examples/status): a self-hosted status page and uptime monitor (HTTP, TCP, DNS and certificate checks, a live dashboard, a public page with 90-day uptime, incidents, webhook, Slack, Discord and email notifications, and badges) built on migrations, jobs, realtime and sessions.
- Background jobs: the `jobs` package stores jobs in SQLite or PostgreSQL, runs them with retries, backoff, timeouts, delays, unique keys and transactional enqueueing, and runs cron and `@every` schedules once across instances. `q.Stats`, `q.List` and `q.Retry` support admin pages.
- Database migrations: the `migrate` package applies `<version>_<name>.sql` files in transactions on startup (`migrate.Plugin`), detects edited migrations, and supports `-- +down` sections. `tern migrate new <name>` creates a file.
- `Setup.OnStart` for plugins that start work with the server, and `App.Start` for custom servers. Start functions don't run while prerendering.
- Realtime: `ctx.EventStream` for server-sent events and `ctx.WebSocket` for WebSockets (same-origin by default), both ending cleanly on shutdown; the `realtime` package's `Hub` publishes to subscribers through a pluggable `Broker`; `stream()` and `socket()` from `@ternkit/tern` keep the latest message in reactive state and reconnect.
- Markdown pages: a `+page.md` route is rendered to HTML by `tern gen`, with its front matter's `title` and `description` in the page head.
- Content collections: the `content` package reads Markdown from `src/content` with typed YAML front matter, rendered with GitHub-flavored Markdown, heading IDs, footnotes and highlighted code. `src/content` is embedded in the binary; drafts show only under `tern dev`. `ctx.Content()` returns the directory.
- `Entries` may take a `ctx *kit.Ctx`, for example to list a collection's slugs.
- A render cache: server-rendered HTML is cached by the page's data and URL (32 MiB by default, `kit.WithSSRCache`), and concurrent renders of the same page are coalesced.
- Rendering VMs are replaced after 10,000 renders, which bounds memory held by libraries across renders.
- More web APIs while server rendering: `TextEncoder`, `TextDecoder`, `atob`, `btoa`, `crypto.getRandomValues`, `crypto.randomUUID`, `structuredClone`, `Event`, `CustomEvent`, `EventTarget`, `AbortController`, `AbortSignal`, `performance.now()`, and timers that never fire. `fetch` rejects with an explanation.
- A compatibility suite renders 23 popular libraries in Tern on every push and checks the HTML matches Node's.
- `tern({ ssrBrowserBuilds })` bundles a package's browser build for server rendering; `nanoid` uses it by default.

### Changed

- The server bundle resolves libraries without the `node` export condition, so they use portable builds instead of ones needing `Buffer` or `node:crypto`.
- Server rendering defaults to at most 4 workers (`kit.WithSSRWorkers` still overrides it). The VMs share a lock in the C runtime, so more workers rendered slower; uncached throughput is up by half.

### Fixed

- Under `tern dev`, the app's own WebSocket endpoints were proxied to Vite; only Vite's connection is now.
- Server rendering no longer leaks memory. The `URL` polyfill stored its state in a way QuickJS never frees, losing about 5 KB per render; Tern now ships its own `URL` and `URLSearchParams` and no longer depends on core-js.

## [1.1.0] - 2026-11-10

### Added

- Responsive images: `import hero from './hero.jpg?image'` encodes AVIF, WebP and fallback versions at several widths with sharp, and `<Image>` from `@ternkit/tern` renders them as a `<picture>` with width, height, `sizes` and lazy loading. `?w=400,800&image` picks the widths.
- Translations: JSON message files in `src/locales`, `t()` with typed keys and ICU plurals and selects in components, `ctx.Locale()` and `ctx.T()` in Go, locale detection from a `lang` parameter, the `locale` cookie or `Accept-Language`, `setLocale()` to switch without a reload, and `%tern.lang%` in `app.html`. The browser loads only the current locale's messages.
- More built-in plugins: `plugins/sessions` (encrypted cookie sessions with flash messages and secret rotation), `plugins/metrics` (Prometheus metrics labeled by route pattern), `plugins/ratelimit` (per-client token buckets) and `plugins/cors`.
- Service workers: `src/service-worker.ts` is built to `/service-worker.js` after prerendering and registered in production. The `$service-worker` module lists the build's assets, static files and prerendered pages, with a version for cache names. `serviceWorker.register` in `tern.json` turns automatic registration off.
- `Setup.TrustOrigin` for plugins that allow cross-site requests.

### Changed

- `ctx.SetCookie` replaces a cookie set earlier in the same request instead of sending both.

## [1.0.0] - 2026-11-09

Tern 1.0 is stable: the APIs described in [docs/stability.md](docs/stability.md) follow semantic versioning from here on.

### Added

- A plugin system: `kit.WithPlugins` and the `kit.Plugin` interface. A plugin's `Setup` can add hooks, middleware, matchers, HTTP handlers served before routes, HTML in every page's `<head>` (with the CSP nonce), and shutdown work.
- The [stability promise](docs/stability.md), and decision records for selective reloading and plugins.
- Public environment variables: `PUBLIC_` variables in the server's environment are available in components as `env` from `@ternkit/tern`, during server rendering and in the browser.
- Base paths: `{"basePath": "/app"}` in an optional `tern.json` serves the app under a URL prefix. Assets, `%tern.assets%`, `route()`, client routing, prerendering and `kit.Redirect` include it; `ctx.Path` and `ctx.BasePath` help with links built in Go.
- Built-in plugins: `plugins/health` (liveness and readiness with checks), `plugins/requestlog` (structured access logs), `plugins/compress` (gzip that keeps streaming working) and `plugins/secure` (security headers).

## [0.4.0] - 2026-11-09

### Added

- `examples/dashboard`: a shadcn-svelte, Tailwind 4, bits-ui, layerchart and SQLite dashboard with sidebar navigation, dialogs, selects, dropdown menus, toasts, charts, streamed data and dark mode, covered by end-to-end tests.
- GET forms (such as search boxes) navigate on the client, keeping focus and scroll when they filter the current page.
- Server rendering supports libraries that read `process.env` or use `Intl` while loading: `process.env.NODE_ENV` is replaced at build time, and the renderer provides a `process` stub and an `en-US` `Intl` (`NumberFormat`, `DateTimeFormat`, `PluralRules`, `Collator`, `RelativeTimeFormat`, `ListFormat`, `Locale`, `Segmenter`).
- A documentation website at <https://unfield.github.io/Tern/>, built from `docs/` by a small generator in `website/` and deployed by GitHub Actions; a logo.
- Tests: a `tern dev` integration test (rebuilds, the error page, recovery, shutdown), a starter-template test that creates, type-checks, builds and runs a project, client unit tests with Vitest, and benchmarks ([docs/benchmarks.md](docs/benchmarks.md)).
- Typed links: `tern gen` writes a `$routes` module, and `route('/projects/[id]', { id })` builds URLs with the pattern and parameters checked by TypeScript.
- Navigation hooks: `beforeNavigate` (can cancel, including leaving the page), `onNavigate` (can delay rendering, for view transitions) and `afterNavigate`.
- Selective reloading: loads record what they read (`ctx.Param`, `ctx.URL`, `kit.Parent`, and keys from the new `ctx.Depends`). On client navigation, layouts whose inputs did not change are reused instead of rerun, and `invalidate(key)` reruns only the loads that depend on `key`.
- Content Security Policy: `kit.WithContentSecurityPolicy` and `kit.WithContentSecurityPolicyReportOnly` add a per-request nonce to `script-src`.
- Prerendered endpoints: a `GET` endpoint with `const Prerender = true`, such as `sitemap.xml`, becomes a static file at build time.
- A redesigned CLI: colored help, a `tern dev` banner with local and network URLs and timestamped rebuilds, `tern build` steps with spinners and a size summary, and styled `routes`, `gen` and `new` output. Color turns off when output is not a terminal; `NO_COLOR` and `FORCE_COLOR` are honored.
- `tern version` also prints the Go version and platform; untagged builds report `dev`.
- If the server bundle fails to load, the error includes the JavaScript stack trace.
- Generated types include Vite's client types, so CSS and asset imports type-check.

### Fixed

- `tern dev` waits for Vite before serving, so the first pages are server-rendered.
- `tern dev` refuses to start when another `tern dev` is already running for the same project, instead of both overwriting the same build output.
- The Vite plugin uses `server.ws.clientPort` instead of the deprecated `server.hmr.clientPort`.
- An action with an `any` result no longer turns every field of `form` into `unknown`.

## [0.3.1] - 2026-11-09

### Changed

- The npm package is now `@ternkit/tern` (it was never published as `@unfield/tern`). Update imports to `@ternkit/tern` and `@ternkit/tern/vite`.

### Fixed

- Build scratch files from `svelte-package` are no longer committed.

## [0.3.0] - 2026-11-09

The first release. It covers the v0.1, v0.2 and v0.3 milestones of the [roadmap](docs/ROADMAP.md).

### Added

- Route scanning: `+page.svelte`, `+layout.svelte` and `+error.svelte`, plus each route directory's Go package (`Load`, `Actions`, `LayoutLoad`, `GET`, `POST`, …). Dynamic directories use Go-safe names: `slug.param`, `id.param.matcher`, `lang.optional`, `path.rest` and `app.group`. All problems are reported at once, including conflicting routes, and SvelteKit-style names get a suggested rename.
- URL matching by route priority, with correct handling of encoded characters and trailing slashes.
- `tern routes` command to list an app's routes.
- `kit`, the server runtime: concurrent layout and page loads with `kit.Parent`, form actions with `kit.Fail` and enhanced JSON responses, endpoints, hooks, middleware, matchers (`integer` and `uuid` built in), cookies with secure defaults, cross-origin request protection, body limits, panic recovery, immutable asset caching with ETags and gzip, and graceful shutdown.
- `tern gen`: generates `tern.gen.go`, the client route manifest, and TypeScript types for every route from the Go types of its loads and actions. Signatures are checked with errors at the source position.
- `@ternkit/tern`, the client runtime and Vite plugin: client-side routing over `__data.json`, `page` and `navigating` state, `goto`, `invalidateAll`, `preloadData`, `enhance` and `applyAction`, scroll restoration, focus management and route announcements.
- `tern dev`: one port for the app and Vite, hot module replacement, automatic Go rebuilds and browser reloads, and an in-browser error page for build errors.
- `tern build`: a single binary with the client embedded and precompressed.
- `tern new`: a starter project that pins the CLI with `go get -tool`.
- `examples/todos` with Playwright end-to-end tests.
- Streaming: `kit.Defer` starts slow work without holding up the page. The value is a `Promise` in Svelte; it streams as an inline script on full page loads and as newline-delimited JSON on client navigations.
- Prerendering: pages with `const Prerender = true` (and an `Entries` function for dynamic routes) are rendered to static HTML and data by `tern build` and served with ETags.
- `tern new` projects include a multi-stage `Dockerfile` producing a distroless image.
- The [guide](docs/guide.md) and the [deployment guide](docs/deployment.md).
- Server-side rendering with hydration. Pages render in a pool of embedded QuickJS VMs (pure Go, no cgo) with per-render timeouts and memory limits, and fall back to client rendering if a render fails. Development renders through Vite. Opt out per page with `const SSR = false` or app-wide with `kit.WithoutSSR()`; tune with `kit.WithSSRWorkers` and `kit.WithSSRTimeout`.

[Unreleased]: https://github.com/Unfield/Tern/compare/v1.1.0...HEAD
[1.1.0]: https://github.com/Unfield/Tern/releases/tag/v1.1.0
[1.0.0]: https://github.com/Unfield/Tern/releases/tag/v1.0.0
[0.4.0]: https://github.com/Unfield/Tern/releases/tag/v0.4.0
[0.3.1]: https://github.com/Unfield/Tern/releases/tag/v0.3.1
[0.3.0]: https://github.com/Unfield/Tern/releases/tag/v0.3.0
