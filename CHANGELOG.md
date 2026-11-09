# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Before v1.0.0, minor versions may contain breaking changes.

## [Unreleased]

### Added

- `examples/dashboard`: a shadcn-svelte, Tailwind 4, bits-ui, layerchart and SQLite dashboard with sidebar navigation, dialogs, selects, dropdown menus, toasts, charts, streamed data and dark mode, covered by end-to-end tests.
- GET forms (such as search boxes) navigate on the client, keeping focus and scroll when they filter the current page.
- Server rendering supports libraries that read `process.env` or use `Intl` while loading: `process.env.NODE_ENV` is replaced at build time, and the renderer provides a `process` stub and an `en-US` `Intl` (`NumberFormat`, `DateTimeFormat`, `PluralRules`, `Collator`, `RelativeTimeFormat`, `ListFormat`, `Locale`, `Segmenter`).
- If the server bundle fails to load, the error includes the JavaScript stack trace.
- Generated types include Vite's client types, so CSS and asset imports type-check.

### Fixed

- `tern dev` waits for Vite before serving, so the first pages are server-rendered.
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

[Unreleased]: https://github.com/Unfield/Tern/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/Unfield/Tern/releases/tag/v0.3.1
[0.3.0]: https://github.com/Unfield/Tern/releases/tag/v0.3.0
