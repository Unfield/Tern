# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Before v1.0.0, minor versions may contain breaking changes.

## [Unreleased]

### Added

- **`tern doctor`** ([docs/deployment.md](docs/deployment.md#before-you-deploy-tern-doctor)) checks a project before a deploy: Go and Node, the Go module's and npm package's versions, code generation, native rendering's fallbacks, the migrations, the target's environment against `.env.example`, HTTPS, security headers and uncommitted changes. It exits 1 when a check fails.
- **Operations in the admin panel** ([docs/admin.md](docs/admin.md#operations)): `acc.UsersAdmin()` and `acc.AuditAdmin()` for accounts and their audit log, `queue.Admin()` for background jobs with Retry, and `admin.Errors(reporter)` for error reports grouped by cause.
- **Actions on records** in the admin panel: `admin.Action` and `admin.DangerAction` add buttons to each record's page that run Go code.
- `acc.SignOutUser` ends every session of an account.
- Shelf's admin panel has its users, audit log and errors.
- The Tern vs SvelteKit benchmark in CI compares each run on `main` with the last one, Tern's binary size included, and warns when Tern is more than 10% slower.

### Changed

- **Secure by default** ([ADR 0027](docs/adr/0027-secure-by-default.md), [docs/security.md](docs/security.md)). These change what a running app sends; each has an option to restore the old behaviour.
  - Every response carries the security headers `plugins/secure` used to add: `X-Frame-Options: DENY`, a strict `Referrer-Policy`, a `Permissions-Policy` that turns off the camera, microphone and location, and `Cross-Origin-Opener-Policy: same-origin`. `plugins/secure` now changes them; `kit.WithoutSecurityHeaders()` drops them.
  - Every page is sent a strict `Content-Security-Policy`: only the app's own scripts run, and inline scripts with the request's nonce, which Tern now also puts on inline scripts in `<svelte:head>`. `kit.WithCSPSources` allows more origins, such as an analytics script; `kit.WithoutContentSecurityPolicy()` sends none. Under `tern dev`, the overlay shows what the policy blocked and the line that allows it.
  - Form actions and other state-changing requests are limited to 120 a minute per client, in bursts of 60; a client over the limit gets 429 with `Retry-After`. Loopback clients aren't limited, and the app warns when requests come through a proxy `kit.WithTrustedProxies` doesn't name. Adding `plugins/ratelimit` replaces the limit; `kit.WithoutRateLimit()` removes it.
- `tern doctor` checks for options that turn protections off, and says what each costs, instead of checking for `plugins/secure`.
- `tern doctor` looks for known vulnerabilities: govulncheck, built in, on the Go modules and Go version, failing for those the app's code reaches, and `npm audit` or `pnpm audit` on the npm packages, warning for high and critical advisories. `-offline` skips them. Tern's own CI runs both.
- **Smaller binaries with native rendering.** The compiled renderer's function descriptors are static data instead of code that runs at startup, modules' state starts out with a loop instead of one assignment per variable, and a binary without the JavaScript engine leaves out the server bundle it no longer needs. The dashboard example's binary goes from 38.9 to 37.4 MB, and starts faster.

### Fixed

- **Native rendering:** a `var` declared in a loop or block at a module's top level no longer crashes the compiler; `\p{ID_Start}` and `\p{ID_Continue}` work in regular expressions; and a sticky or global regular expression after non-ASCII text starts where `lastIndex` says, where it started too early before.

## [1.15.0] - 2026-11-12

### Added

- **Admin screens** ([ADR 0024](docs/adr/0024-admin-screens.md), [docs/admin.md](docs/admin.md)): the `admin` package generates screens to manage an app's data from Go structs.
  - For each kind of record, a list with search, filters, sorting and pages, and a form to create, edit and delete one, checked against the struct's `validate` tags and its `Validate` method.
  - `admin.NewTable` keeps records in a SQLite or PostgreSQL table; `admin.Store` is for data kept elsewhere.
  - The `admin` tag marks keys, listed, searchable and filtered fields, read-only fields, references to other resources, and integers that hold times.
  - Tern UI's `Admin` component renders them, in a route of the app's own, so they work without JavaScript.
- Shelf, the tutorial's app, has admin screens for its links and accounts.
- **`tern add`** ([guide](docs/guide.md#plugins)): adds plugins to the app's `main.go`, by name or any Go package with a plugin, keeping its formatting and comments; it gets the module when it isn't Tern's own. **tern.land/plugins** lists Tern's plugins and the community's, from `plugins/registry.json`, where a test checks that every entry compiles.
- **Editors** ([docs/editors.md](docs/editors.md)): `tern lsp` is a language server that goes from a page's `data` to its Go load and back, finds routes as workspace symbols, and shows `tern check`'s errors when files are saved. Extensions for VS Code (with a Routes view) and Zed start it; Neovim and Helix can too.
- **Collaboration in live components** ([docs/live-components.md](docs/live-components.md#collaboration)): `goscript.Room` shows who is looking at the same thing, each with state of their own, such as which field they're in; `goscript.Docs` is a shared record that people change together, merged field by field, loaded and saved through functions of the app's own. Both work across server processes through the broker, and `tern check` types them in the markup.
- The goscript example has a shared note.
- **Feature flags** ([docs/flags.md](docs/flags.md)): the `flags` package turns features on for some users first, decided on the server: a kill switch, users and teams who always get a feature, and a rollout to a share of users, teams or visitors that keeps everyone who had it as it grows. Flags live in the database and are read from memory; `f.Admin()` adds a screen for them to the admin panel, and the dev tools show which flags each page checked.
- Shelf has a `search` flag, on for everyone, and a screen for its flags.
- **Search** ([ADR 0025](docs/adr/0025-search.md), [docs/search.md](docs/search.md)): the `search` package indexes an app's data in SQLite (FTS5), PostgreSQL (`tsvector`) or memory, and finds it as people type: words match as prefixes, without case or accents, within a scope such as one user's documents. Hits come with snippets that mark the matches, as text rather than HTML.
- **`SearchBox` and `Highlight` in Tern UI:** results as the user types, with keyboard navigation and a hotkey, and a results page that works without JavaScript.
- `admin.AfterSave` and `admin.AfterDelete`, to keep a search index or a cache in step with what admins change.
- Shelf searches each user's links.
- **Streaming endpoints** ([guide](docs/guide.md#streaming-endpoints)): `func POST(ctx *kit.Ctx, in Ask, out *kit.Stream[Delta]) error` sends values as they come, such as a model's reply as it's written, as server-sent events. `$api`'s `stream` reads them as a typed async iterator, and throws the server's error as an `ApiError`. Leaving the loop stops the endpoint. The OpenAPI document describes them.
- **Dev tools in `tern dev`** ([docs/dev-tools.md](docs/dev-tools.md)): a button on every page opens a panel with the route, its files at the line of each load, the data each load returned, and each request's spans and logs. Errors from the server, the browser and the build open over the page, with the app's frames, the source around each and a button that opens it in your editor (`TERN_EDITOR`, or Zed, Cursor or VS Code).

### Changed

- **Native renders allocate less:** strings and numbers become values from slabs rather than one box each, on the paths that make most of them (HTML, page data, string helpers). The 100-row table renders with 40% fewer allocations (5,501 to 3,285) and 5% faster (268 to 254 µs).

## [1.14.0] - 2026-11-12

### Added

- **Apps for assistants** ([ADR 0023](docs/adr/0023-apps-over-mcp.md), [docs/mcp-server.md](docs/mcp-server.md)): the `mcp` package serves an app's tools to AI assistants over MCP, at `/mcp`.
  - Tools are Go functions with typed arguments, whose `json` names, `description` tags and `validate` rules become their schema and are checked before they run. They run as the signed-in user.
  - It speaks MCP 2026-07-28 and the 2025 versions on one endpoint, without sessions.
  - With `plugins/oidc`, users approve each assistant on the app's consent screen, and tokens are bound to the MCP server.
- **`plugins/oidc`:** Client ID Metadata Documents (`ClientMetadataDocuments`), so clients such as assistants register themselves, fetched with protection against server-side request forgery. Resource indicators (RFC 8707), with `AccessToken.For` to check a token's audience. Authorization server metadata at `/.well-known/oauth-authorization-server` (RFC 8414).
- `kit.ValidateInput` and `Ctx.UserError`, for code that answers requests its own way.
- **A tutorial** on tern.land that builds a complete app, one step per page: a reading list with a database, validated forms, accounts, tests and a deploy. The finished app is [examples/shelf](examples/shelf).

### Fixed

- **Security:** `plugins/oidc` skipped the consent screen for an app that asked for no scopes, since "every requested scope was approved" was true of none. Consent is now always needed once per app, unless it's `Trusted`.
- `npm test` in a new project works before its first build: it runs `tern build`, which the tests need to render pages, instead of `tern gen`.

## [1.13.0] - 2026-11-12

### Added

- **Smaller binaries:** when every page compiles to Go, `tern build` leaves the JavaScript engine out, about a tenth of the binary. `TERN_KEEP_ENGINE=1` keeps it.
- **Emails render natively,** as pages do, with the engine as the fallback. Live components already did, within their pages.

### Fixed

- Error groups are listed most recent first on Windows too, where two reports moments apart could get the same time.

## [1.12.0] - 2026-11-12

### Added

- **Rendering on demand** ([ADR 0021](docs/adr/0021-rendering-on-demand.md), [docs/caching.md](docs/caching.md)):
  - **Pages:** `const Revalidate = 300` renders a page on its first request and serves it from the cache, rendering it again in the background once it's stale. Cached pages are rendered without the visitor's cookies, so nothing personal can be cached.
  - **Tags:** `c.Tag` names the data a page used, and `c.Revalidate(tags...)` refreshes everything that used it, in every process.
  - **The `cache` package:** in memory, on disk or in Redis (`cache/redis`, with no dependency), with tags, stale-while-revalidate and one call per key at a time. `cache.Get` keeps load results in `c.Cache()`.
  - **Cache headers:** `const CacheControl` sets `Cache-Control` for a route's pages and data, for CDNs.
  - **No JavaScript:** `const CSR = false` sends a page without scripts.

## [1.11.0] - 2026-11-12

### Added

- **Teams in `accounts`** ([ADR 0019](docs/adr/0019-teams.md), [docs/accounts.md](docs/accounts.md#teams)):
  - **Teams:** users make teams, belong to several and switch between them. Each session remembers the current one: `accounts.CurrentTeam` and `acc.RequireTeam`.
  - **Invitations by email:** only the invited address can accept, and accepting verifies it.
  - **Roles:** owner, admin and member are built in, with fixed rules, and a team never loses its last owner. Apps add roles of their own and check them with `rbac` through `accounts.TeamRoles`.
  - **The app's data:** `Options.OnDeleteTeam` deletes it before a team goes.
- **An audit log in `accounts`:** sign-ins and failed ones, password, email and two-step changes, passkeys, sessions, and team and member events, with the IP and device. Users see their own (`acc.Activity`), team admins their team's (`acc.TeamActivity`), and support tools anyone's (`acc.AuditEntries`). Apps add events with `acc.Audit`.
- **Team screens in Tern UI:** `NewTeam`, `TeamSwitcher`, `TeamMembers`, `TeamInvitations`, `TeamSettings`, `AcceptInvitation` and `ActivityLog`.
- **The saas template has teams:** everyone gets a team at sign-up, projects belong to the team, and `/settings/team` has the members, invitations and the team's activity. The security page shows the user's own activity, and the dashboard counts members.
- **Billing with Stripe** ([ADR 0020](docs/adr/0020-billing.md), [docs/billing.md](docs/billing.md)):
  - **Plans in Go**, with Stripe's prices, trials, seats per member, limits and features.
  - **Subscribing:** `b.Checkout` starts Stripe Checkout, and `b.Portal` opens the customer portal.
  - **Webhooks:** verified, recorded once, and processed as jobs that fetch the subscription from Stripe, so repeated, late and out-of-order events are safe.
  - **Limits:** `b.Check` and `b.Has` check a plan's limits and features in loads and actions, with messages for forms and a 402 for endpoints.
  - **Screens:** Tern UI's `PricingTable` and `BillingSettings`.
- **The saas template bills teams:** a free plan with limits and a per-member Pro plan with a trial, a `/pricing` page, `/settings/billing` with the usage, and seats kept in step with members. Without Stripe's key, every team is on the free plan.
- `kittest.Client.Browser` returns a second client of the same app, with no cookies of its own.

### Fixed

- Link buttons in Tern UI's sign-in and account cards kept the cards' link color, so their text was hard to read.
- `scripts/test-template.sh` no longer stops the shell that runs it when a step fails before the server starts.

### Changed

- `accounts.Migrate` creates the teams and audit tables, and adds a `team_id` column to `tern_sessions`.

## [1.10.0] - 2026-11-11

### Added

- **Tracing** ([ADR 0017](docs/adr/0017-tracing.md), [docs/tracing.md](docs/tracing.md)):
  - **Spans:** the `tracing` package records requests as trees of timed spans and exports them over OTLP to any OpenTelemetry backend.
  - **kit:** `kit.WithTracer` makes kit trace each request, with spans for its loads, actions, endpoints and server render, and it continues a trusted `traceparent`.
  - **HTTP and SQL:** `tracing.Transport` traces outgoing HTTP requests and passes the trace on. `tracing.OpenDB` traces SQL queries.
  - **Jobs:** a job runs in the trace it was queued in.
  - **Logs:** records logged inside a span carry its trace and span IDs.
  - **Sampling:** `TERN_TRACE_SAMPLE` keeps a fraction of traces.
- `tern dev` traces every request and shows each one's spans as a waterfall in the log view.
- **Error reports** ([ADR 0018](docs/adr/0018-error-reports.md), [docs/errors.md](docs/errors.md)):
  - **What's reported:** unexpected errors and panics from loads, actions, endpoints, hooks, live components and jobs, plus uncaught errors and unhandled rejections in the browser.
  - **Grouped by cause** and rate-limited, with the stack, request, route, trace, release and user.
  - **Sent to Sentry** or any service that takes its protocol.
  - **Wiring:** `kit.WithReporter` turns it on. `report.WithStack` and `report.Errorf` give returned errors a stack.
  - **In dev:** `tern dev`'s log view lists the groups.
- **Testing** ([docs/testing.md](docs/testing.md)):
  - **Loads and actions:** `kit/kittest` calls loads, actions and endpoints from Go tests, with a request made of options (path, form, JSON, route parameters, layout data) and the app's hooks run. Helpers read redirects, failures and statuses.
  - **A database per test:** `kittest.SQLite` gives each test its own in-memory database, migrated.
  - **Pages:** `kittest.NewClient` runs the whole app and queries its HTML with CSS selectors.
  - **Signed-in users:** `accounts.ActAs` makes a user the signed-in one for a request.
  - **Building blocks:** `kit.App.NewCtx` makes a request's Ctx, which kittest builds on.
- New projects come with tests: Go tests of their pages (`npm test`), and Playwright set up with a browser test of the home page (`npm run test:e2e`).

### Changed

- The `tern_jobs` table has a `trace` column, which `Migrate` adds to existing databases.

## [1.9.0] - 2026-11-11

### Added

- **Logging** ([ADR 0014](docs/adr/0014-logging.md), [docs/logging.md](docs/logging.md)). `logging.New` returns a `*slog.Logger` with these features:
  - Request context: kit gives every request an ID, kept from a trusted `X-Request-Id` or made new, and returns it as `X-Request-Id`. Records logged with the request's context include it, plus `trace_id` and `span_id` from a trusted `traceparent`.
  - `c.Logger()` binds the ID and the route; `c.LogWith` and `logging.With` add attributes of your own.
  - Passwords, tokens, keys, cookies and other secrets are redacted by key, including in headers and URLs, and `logging.Secret` always is.
  - Optional sampling of floods.
  - `TERN_LOG_LEVEL` and `TERN_LOG_FORMAT`.
- Logging sinks that ship records in batches with retries, without blocking: `logging/otlp` (OpenTelemetry over HTTP), `logging/loki`, `logging/datadog` and `logging/file` (JSON lines, rotated). A sink of your own is one method.
- **HTTPS without a proxy:** `kit.WithAutoTLS`, or `TERN_TLS_DOMAINS`, serves HTTPS with certificates from Let's Encrypt, which the app gets and renews itself, and redirects plain HTTP. `kit.WithTLS` takes certificates of your own. HTTP/2 is offered in both cases.
- **Security signals** ([ADR 0015](docs/adr/0015-security-signals.md), [docs/signals.md](docs/signals.md)): `plugins/signals` scores how automated each request looks, with reasons, from:
  - the JA4 TLS fingerprint, an HTTP/2 fingerprint and the order of the headers;
  - how the headers fit the browser the User-Agent claims;
  - an optional client check in the browser: a proof of work and three automation tells.
  It also gives each browser a device ID. `signals.Of(c)` returns them, `Options.Block` refuses scripted form posts, and `signals.Key` makes rate limits per client.
- `accounts`: `Options.NotifyNewDevices` emails users about sign-ins from a device their account hadn't been used on, and `OnNewDevice` is called then.
- kit plugins can wrap connections (`Setup.WrapConn`), see TLS ClientHellos (`Setup.OnClientHello`) and add to a connection's context (`Setup.ConnContext`). `kit.IsHTTPS` and `kit.FromTrustedProxy` are exported.
- **`tern deploy`** ([ADR 0016](docs/adr/0016-tern-deploy.md), [docs/deployment.md](docs/deployment.md#tern-deploy)) deploys to targets named in `tern.json`:
  - `ssh` puts the app on a Linux server under systemd, creating its user and unit, and serves HTTPS itself when the target has domains. Restarts have zero downtime: the new release starts next to the old one on the same port, and the old one stops only once the new one is ready. Releases are kept for `-rollback`, and `-releases` and `-logs` work too.
  - `fly` deploys to Fly.io.
  - `container` builds and pushes an image, then runs the platform's deploy command.
  - Any other provider runs a `tern-deploy-<name>` program with the deployment as JSON.
- kit tells systemd when the app is ready and when it stops (`Type=notify`), and with `TERN_REUSEPORT=1` listens with SO_REUSEPORT, so two releases can share a port.
- `tern build` cross-compiles apps that prerender pages: with GOOS or GOARCH set, it prerenders with a binary for the machine it runs on.
- `tern dev` has a live log view at `/__tern/dev/logs`: the app's records and the requests it serves, filterable by level, text and request.

### Changed

- kit adds the request's attributes to records from loggers it didn't make, and flushes the logger's sinks on shutdown. The saas template and the examples log with `logging.New`.

### Fixed

- `tern dev` serves files from the workspace's `node_modules`, such as fonts from `@fontsource`, in pnpm and npm workspaces. Tern's Vite plugin allowed only the app's own folder.
- `tern dev` no longer shows pages unstyled on their first paint. Vite delivers stylesheets as JavaScript after the HTML; the dev server now puts the CSS of the page's components in the server-rendered HTML, and the client drops that copy once Vite's styles, which hot-reload, are in place.
- Scroll positions: a page opened in a new history entry, from a link elsewhere or the address bar, starts at the top instead of at a position saved for an earlier page. Moving between pages jumps there at once, even where the CSS sets `scroll-behavior: smooth`: the animation used to stop partway as the new page rendered.

## [1.8.0] - 2026-11-11

### Added

- **A full app to start from.** `tern new -template saas`, or "full" when `tern new` and `npm create tern` ask, creates an app with accounts built: sign-up, email verification, password resets, passkeys, two-step sign-in, and sign-in with GitHub or Google when their keys are set. It has a landing page, a dashboard with a getting-started checklist, projects as an example of per-user data in SQLite with a migration, settings for the profile, email address and password, and a security page for two-step sign-in, passkeys, devices and connected accounts. Emails go through SMTP in background jobs, or to the log, link included, in development. The bare templates are as before.
- `tern dev` reads a `.env` file into the app's environment; variables already set win over it.

### Changed

- **Native rendering, phase 2** ([ADR 0013](docs/adr/0013-native-rendering-phase-2.md)): an uncached render of the benchmark's 100-row table takes 272 µs, from 352 µs in v1.7.0, and allocates 352 KB, from 573 KB.
  - Page data reaches native renders as the loads' Go values, made into the same JavaScript values `JSON.parse` would make, without the JSON. Don't change what a load returns after it returns.
  - Svelte's `Renderer.push` and `#collect_content`, and its `attr`, `to_class`, `attr_class`, `stringify` and `ensure_array_like` helpers, run as Go. Each is used only while Svelte's source is the one it was written against, and is checked against Node.
  - The document is written in one pooled buffer instead of being copied at each step.

### Fixed

- Native rendering: String objects have their index properties, so `Object.keys('ab')` is `['0', '1']`.

## [1.7.0] - 2026-11-11

### Added

- **Two-step sign-in** in `accounts`: codes from an authenticator app (TOTP) after the password, with recovery codes. `SetUpTwoStep` returns a QR code, `ConfirmTwoStep` turns it on, `SignIn` returns `ErrTwoStepRequired` and `CompleteSignIn` takes the code. Tern UI's `TwoStep` and `TwoStepCode` blocks show it.
- **Sign in with GitHub, Google or OpenID Connect into `accounts`:** `plugins/oauth` takes `Accounts: acc` in place of `OnLogin`. A provider's sign-in joins the account with the same verified email, or creates one, or connects to the signed-in user; `acc.Identities` and `acc.Unlink` manage them, and Tern UI's `LinkedAccounts` shows them. `acc.SignInWith` does it for any provider.
- `acc.RequestEmailChange` changes a user's email address once they open a link sent to the new one, and tells the old one.
- `accounts` limits attempts per account and per IP, on by default: sign-ins, sign-ups, emails, passkey sign-ins and two-step codes. Past a limit, methods return `ErrTooManyAttempts`; `Options.NoRateLimits` turns them off, for tests.
- A Benchmarks workflow: every push and pull request renders the benchmark app's pages at the commit before and at the new one, and fails on a significant slowdown of more than 10%. Pushes to `main` also run the comparison with SvelteKit.
- `tern routes -json` prints the routes as JSON: pattern, directory, kind, whether it has a load, and its layouts.
- `tern dev` says which routes were added and removed when route directories change.

### Changed

- Page data, action results, typed endpoint output and live component state are encoded by Tern's own JSON encoder: byte for byte what encoding/json writes, with nil slices as `[]`, about 6× faster, without copying the data first. An uncached render of the benchmark's 100-row table takes 17% less time.
- `tern build` compiles the main package, which holds the native renderer, without inlining: the dashboard example's binary is 11% smaller (45 → 40 MB) and builds faster, with renders as fast as before.
- Native rendering builds the page data's objects at their final shape, and property reads that miss their cache no longer allocate.

### Fixed

- Tern UI's `SignIn` said a sign-in had failed when the user cancelled it at the provider: it now matches `plugins/oauth`'s `denied` code.
- Tern UI's `Form` hid the server's error on a field that lost focus without being changed, and two submits in a row raced each other: now the newest submit aborts the older one.

## [1.6.0] - 2026-11-11

### Added

- **User accounts.** The new `accounts` package signs users up and in with a password or a passkey, and keeps their accounts in your database (SQLite or PostgreSQL). See [docs/accounts.md](docs/accounts.md) and [ADR 0012](docs/adr/0012-accounts.md).
  - Email verification and password reset links, sent with `accounts.SendWith(mailer, queue)` or your own `Send`, in the user's language.
  - Sessions in the database: `acc.Sessions` lists the devices a user is signed in on, and `acc.Revoke` and `acc.SignOutOthers` sign them out. Changing or resetting a password signs out the others.
  - Passkeys (WebAuthn, ES256, EdDSA and RS256): `registerPasskey()` and `signInWithPasskey()` from `@ternkit/tern`, over endpoints the plugin mounts.
  - Passwords hashed with argon2id. bcrypt hashes from another system still work and are rehashed at the next sign-in.
  - `accounts.Current(ctx)` and `acc.Require(ctx)` in loads and actions. `accounts.FormError` turns a wrong password or a taken email into a form error.
  - Tern UI blocks for each screen: `ForgotPassword`, `ResetPassword`, `Devices` and `Passkeys`, and a passkey button on `SignIn`.
  - [examples/accounts](examples/accounts) runs every flow, passkeys included, in browser tests.
- **Tern's own texts through `src/locales`.** Tern UI's labels and the validation messages are messages with keys starting with `tern.`, with English built in. Translate them in your locale files like your own messages; what a locale leaves out stays English. `kit.FormAction` and `validate()` translate validation messages for the request's locale, so the server and the browser agree. The guide lists every key.
- Native rendering compiles everything that was left to the JavaScript engine: dynamic `import()`, BigInt, async generators, `for await` and `new.target`. It also has every built-in QuickJS has: `Object.groupBy` and `Map.groupBy`, the Set methods, iterator helpers and `Iterator`, `WeakRef` and `FinalizationRegistry`, `Array.prototype.copyWithin`, `String.prototype.isWellFormed` and `toWellFormed`, `Symbol.keyFor`, `escape` and `unescape`, and the RegExp `d` (match indices) and `v` flags. No example leaves anything to the engine.

### Changed

- A nil Go slice is sent as `[]` instead of `null` in load data, action results, deferred values, typed endpoint output and live component state, so slices are typed `T[]` instead of `T[] | null`.
- `Combobox`, `SelectMenu`, `DatePicker`, `Slider`, `Switch`, `Toggle` and `ToggleGroup` are checked as the user changes them inside a `Form`, like inputs.
- A page whose render throws an error that the JavaScript engine throws too keeps rendering natively. Only a page native rendering can't do goes to the engine from then on.
- Renders are faster: the benchmark table renders in 259 µs, from 320. The page data is only searched for deferred values when it has some, and native renders return their result without encoding it as JSON.

## [1.5.0] - 2026-11-11

### Added

- **Native server rendering.** `tern build` compiles the server renderer (your components, Tern's runtime, Svelte's server runtime and libraries) to Go, so pages render without the JavaScript engine: the benchmark app's 100-row table renders in 0.3 ms instead of 2.9 ms, and serves 12× more uncached requests per second, more than SvelteKit on ten Node processes. Renders run in parallel on every core.
  - The HTML is byte for byte what the JavaScript engine renders. What the compiler doesn't support (async generators, `for await`, dynamic `import()`, BigInt) is listed in `.tern/native/report.txt`; pages that use it, and any page whose native render fails, render with the engine as before.
  - `TERN_NATIVE_VERIFY=1` renders every page both ways, serves the engine's HTML and logs differences. `TERN_NATIVE=0`, `"native": false` in `tern.json` or `kit.WithoutNativeSSR()` turn it off.
  - Add `tern_native.gen.go` to `.gitignore`: it is generated by every build.

### Changed

- Server rendering reuses the page data it parsed instead of copying it, in the browser and the JavaScript engine too.
- `kit.WithSSRWorkers` defaults to one worker per CPU when pages render natively. The JavaScript engine still uses at most 4.

## [1.4.0] - 2026-11-10

### Added

- **Form validation from Go struct tags.** `kit.FormAction` decodes a post into a struct and checks `validate:"required,min=3,email,…"` before the action runs, failing with a `kit.Invalid` of errors by field and the posted values (never passwords). `tern gen` writes the same rules to `./$forms`, `validate()` from `@ternkit/tern` checks them in the browser with the same messages, and Tern UI's new `Form` checks them as the user types and shows each error on its `Field` by `name`. `kit.FieldError` and `kit.FormError` fail with a message on a field or on the whole form. Shared test cases keep Go and TypeScript in step.
- **The complete Tern UI.**
  - Overlays: `Popover`, `HoverCard`, `AlertDialog`, `Sheet`, `Drawer`, `ContextMenu` and `Menubar`. Menus gain submenus, checkbox and radio items, shortcuts and labelled groups.
  - Inputs: `Combobox` (with server search), `SelectMenu`, `Command` (and a ⌘K palette), `Calendar`, `DatePicker` (with ranges), `Slider` (with ranges), `PasswordInput`, `InputOTP`, `Toggle`, `ToggleGroup`, `InputGroup`, `ButtonGroup` and `Label`.
  - Layout and data: `Sidebar`, `NavigationMenu`, `Breadcrumb`, `Pagination`, `DataTable`, `Table`, `Resizable`, `Carousel`, `Collapsible`, `ScrollArea`, `AspectRatio`, `Empty` and `Item`.
  - Charts: `LineChart`, `AreaChart`, `BarChart`, `DonutChart`, `Stat` and `Sparkline`, with a palette validated for color blindness (`--tern-chart-1` to `--tern-chart-8`), values read from the keyboard, and a table view.
  - Blocks: `SignIn` (OAuth providers and a password form), `SignUp` and `ConsentScreen`.
- **`tern ui add <Component>`** copies Tern UI components, and the helpers they use, into `src/lib/ui` to change them; `tern ui list` lists them. The MCP server has it as `ui_add`.
- **`npm create tern@latest`** (the new `create-tern` package) runs the matching `tern new`. `tern new` without a directory asks where to create the project, which template to use, whether to style it with Tern UI, and whether to create a git repository and run `npm install`; `-ui`, `-git` and `-install` do the same without questions, and flags may follow the directory.
- **`examples/analytics`:** a self-hosted, privacy-friendly web analytics app. A cookie-free tracking script, a live dashboard with charts and tables, sign-in, hourly totals and pruning as jobs, a weekly email report, and a stats API, in one binary.
- `ctx.ClientIP()` and `kit.ClientIP(r)` return the client's address. `kit.WithTrustedProxies` names the reverse proxies whose `X-Forwarded-For` is believed, read from the right so clients can't spoof it. The rate limiting plugin uses it.
- `plugins/oidc`: `ConsentURL` sends users to a consent page of the app, and `ConsentRequest` returns what it shows, checked as the authorize endpoint checks it.
- The server renderer's `Intl.NumberFormat` supports `signDisplay` and `compactDisplay: 'long'`, matching Node's `en-US` output.
- The guide has recipes for signing in with a password and for validating forms.

### Changed

- Typed endpoints check `validate` tags on their input, as `kit.FormAction` does, and answer 400 when they fail. Inputs with such tags were accepted unchecked before.
- Tern UI's menus and tooltips open in the top layer and stay on screen, so containers with `overflow` no longer clip them. `Tooltip` takes `left` and `right` too.
- Once `kit.WithTrustedProxies` is set, `X-Forwarded-Proto` is only believed from those proxies.
- Compact numbers during server rendering round like browsers do (`12K`, not `12.3K`; `999,999` is `1M`).

### Fixed

- A `Field` in a stretched grid row no longer grows its label, so controls in a row line up.
- Two compiled `tern` binaries had been committed to the repository by accident; they are removed and ignored.
- The guide's hook example called `sessions.User`, which doesn't exist.

## [1.3.0] - 2026-11-10

### Added

- Live components are supported, with no flag: `<script lang="go" live>` runs a component's Go on the server. See [docs/live-components.md](docs/live-components.md).
  - **Concurrency:** each component runs on its own goroutine, so components run in parallel and a slow Func delays only its own. Work from other goroutines goes through `c.Do`, `c.Go` and `c.Every`.
  - **Funcs:** they may take a `context.Context` and return a value and an error. `goscript.Fail` errors reach the page; other errors and panics are logged and reported generically.
  - **In the markup:**
    - each Func has `.pending`, `.error`, `.optimistic(values, …)` and `.debounce(ms)`
    - `live` gives the connection state, bindings (`bind:value={live.bind.name}` for a State declared with `goscript.Bind`) and props
  - **Props and events:** `goscript.Props[T]` follows the parent's props without remounting, and `c.Emit` calls the parent's `on…` handlers.
  - **Access control:** `c.Reject` refuses a mount, such as for visitors who aren't signed in. It works during server rendering too.
  - **Updates and reconnects:**
    - changes go in one message per batch, with patches for lists and objects
    - pages reconnect with backoff, and `Bind` and `Restore` values survive reconnects and deploys
  - **Sharing across processes:** `goscript.Shared`, `goscript.Presence` and `goscript.Publish`/`Subscribe` reach every process through a broker, set with `kit.WithLiveBroker`. `realtime/pgbroker` is a broker on PostgreSQL LISTEN/NOTIFY.
  - **Limits:** `kit.WithLive` sets limits: instances per page, call rate, send buffer and ping interval.
  - **Testing:** `goscript/livetest` tests components in Go without a browser.
- `tern check` generates code and runs `svelte-check`, with the markup of Go components checked against types generated from their Go code. Starter templates' `npm run check` uses it.
- The status example's admin dashboard is a live component.
- `@ternkit/tern/ui`, a component library with Tern's own design, themeable with CSS custom properties, in light and dark mode. See [docs/ui.md](docs/ui.md).
  - **Components:** Button, Field with Input, Textarea and Select, Checkbox, RadioGroup, Switch, Dialog, Tabs, Menu, Tooltip, Accordion, toasts, Alert, Progress, Spinner, Card, Badge, Avatar, Skeleton, Separator and Kbd.
  - **Accessibility:** they follow the WAI-ARIA patterns and work with the keyboard, and `examples/ui` audits them with axe in CI.
- `tern mcp` serves the Model Context Protocol for AI coding tools. Its tools cover project information, routes and their types, live components, code generation, checks, tests, builds, scaffolding routes, live components and migrations, and Tern's docs for the version in use. New projects include a `.mcp.json`. See [docs/mcp.md](docs/mcp.md).

### Fixed

- Server rendering looped forever on some pages with two-way bindings: the minified server bundle was not strict mode, so a renamed block-level function could shadow a variable. The server and service worker bundles are now strict.

### Changed

- Plugins' hooks run before the app's own hooks, so app hooks can use sessions and other plugin state, as the guide's example does.
- `tern dev` rebuilds the server only when a live component's Go code changed, not on markup edits.
- Breaking, for the experimental Go component API:
  - `GlobalSignal.Get` no longer subscribes; read the signal `Show` or the new `Use` returns.
  - `goscript.MountLive` takes options.
  - `Effect` no longer runs during server rendering.

## [1.2.1] - 2026-11-10

### Changed

- `@ternkit/tern` is published to npm with [trusted publishing](https://docs.npmjs.com/trusted-publishers) and provenance; the release workflow holds no npm token.

## [1.2.0] - 2026-11-10

### Added

- Translated URL paths: `"$routes": {"projects": "projekte"}` in a locale file makes `/de/projekte` match `/lang.optional/projects`, and `route()` builds localized paths when given `lang`.
- Remote images: `<Image src="https://…" width height>` with `plugins/images`, which fetches images from allowed hosts, resizes them to the srcset's widths and caches them.
- `tern new -template blog|api`: a Markdown blog with prerendered posts and an RSS feed, or typed JSON endpoints with OpenAPI and a typed client. CI builds and runs every template.
- [Coming from SvelteKit](docs/migrating-from-sveltekit.md): a guide that maps SvelteKit's files, APIs and modules to Tern's.
- Experimental live Go components, `<script lang="go" live>`: the same `goscript` API running on the server over one WebSocket per page, server-rendered with real state per request (props and `Locals`), with `goscript.Global` state shared across visitors, `OnDestroy` and `Rendering`.
- Experimental `<script lang="go">` (`"experimental": {"goScript": true}` in `tern.json`): component logic in Go with runes (`goscript.State`, `Derived`, `Effect`, `Func`), compiled to one WebAssembly module per app, with build-time initial state for server rendering. See [docs/experimental-go-script.md](docs/experimental-go-script.md) for measurements and limits.
- Server rendering formats `Number.prototype.toLocaleString` and the `Date` locale methods like browsers (en-US), so their output hydrates.
- Typed endpoints: `func GET(ctx *kit.Ctx, in Query) (Out, error)` decodes the query, path parameters (`path:"id"`) or JSON body, runs an optional `Validate()`, and writes the output as JSON. `tern gen` writes an OpenAPI 3.1 document of them (served with `kit.WithOpenAPI`) and a typed client, `api('POST /api/projects', { body })` from `$api`, which throws `ApiError` on failure.
- File storage: the `storage` package stores files on disk or in S3-compatible buckets (SigV4, presigned URLs), receives uploads with size limits and content sniffing (`storage.Receive`), makes unguessable keys, and serves files with expiring signed links, range requests and safe headers (`storage.Serve`).
- `ctx.SetBodyLimit` raises the body limit for one request, such as an upload.
- Email: Svelte components in `src/emails` render on the server into a subject, HTML with the component's styles, and a plain-text version (`app.RenderEmail`). The `mail` package sends them over SMTP (STARTTLS or implicit TLS), with a `Log` transport for development and `mail.Job` for retries. Under `tern dev`, `/__tern/emails` previews every template with its `preview` props.
- `plugins/oauth`: sign in with GitHub, Google or any OpenID Connect provider, with PKCE, state and nonce in an encrypted cookie and verified ID tokens.
- `plugins/oidc`: an OAuth 2.0 authorization server and OpenID Connect provider with discovery, JWKS, consent, the authorization code flow with PKCE, refresh token rotation with reuse detection, client credentials, userinfo, revocation and introspection, RS256 JWT access tokens and `provider.Bearer` for APIs. Its state lives in a `MemoryStore` or a `SQLStore` (SQLite, PostgreSQL).
- `rbac`: roles, permissions with wildcards and included roles, `Require`, `Can` and a hook for whole sections.
- `Setup.Endpoint` (plugin routes with a `Ctx`, after hooks) and `Setup.AllowCrossOrigin`.
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

[Unreleased]: https://github.com/Unfield/Tern/compare/v1.15.0...HEAD
[1.15.0]: https://github.com/Unfield/Tern/releases/tag/v1.15.0
[1.14.0]: https://github.com/Unfield/Tern/releases/tag/v1.14.0
[1.13.0]: https://github.com/Unfield/Tern/releases/tag/v1.13.0
[1.12.0]: https://github.com/Unfield/Tern/releases/tag/v1.12.0
[1.11.0]: https://github.com/Unfield/Tern/releases/tag/v1.11.0
[1.10.0]: https://github.com/Unfield/Tern/releases/tag/v1.10.0
[1.9.0]: https://github.com/Unfield/Tern/releases/tag/v1.9.0
[1.8.0]: https://github.com/Unfield/Tern/releases/tag/v1.8.0
[1.7.0]: https://github.com/Unfield/Tern/releases/tag/v1.7.0
[1.6.0]: https://github.com/Unfield/Tern/releases/tag/v1.6.0
[1.5.0]: https://github.com/Unfield/Tern/releases/tag/v1.5.0
[1.4.0]: https://github.com/Unfield/Tern/releases/tag/v1.4.0
[1.3.0]: https://github.com/Unfield/Tern/releases/tag/v1.3.0
[1.2.1]: https://github.com/Unfield/Tern/releases/tag/v1.2.1
[1.2.0]: https://github.com/Unfield/Tern/releases/tag/v1.2.0
[1.1.0]: https://github.com/Unfield/Tern/releases/tag/v1.1.0
[1.0.0]: https://github.com/Unfield/Tern/releases/tag/v1.0.0
[0.4.0]: https://github.com/Unfield/Tern/releases/tag/v0.4.0
[0.3.1]: https://github.com/Unfield/Tern/releases/tag/v0.3.1
[0.3.0]: https://github.com/Unfield/Tern/releases/tag/v0.3.0
