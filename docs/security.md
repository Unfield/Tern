# Security

Tern turns protections on without being asked, and turning one off takes an option with its name on it ([ADR 0027](adr/0027-secure-by-default.md)). `tern doctor` lists the ones an app has turned off, and what each costs.

## On by default

| Protection | What it stops | Turned off with |
|---|---|---|
| Routes closed by default | a route with server code that nobody said who may use: it doesn't build | — |
| Cross-origin request protection | other sites posting forms to the app, or calling its endpoints with the user's cookies | `kit.WithoutCSRFProtection()` |
| Cookies `SameSite=Lax` and `Secure`, and session cookies `HttpOnly` | cookies sent with cross-site requests or over plain HTTP, and scripts reading sessions | — |
| Security headers | other sites framing the app (clickjacking), full URLs leaking in referrers, pages using the camera, microphone or location | `kit.WithoutSecurityHeaders()` |
| Content-Security-Policy | injected markup running scripts (cross-site scripting) | `kit.WithoutContentSecurityPolicy()` |
| Rate limit on writes: 120 a minute per client, in bursts of 60 | scripts hammering forms and endpoints | `kit.WithoutRateLimit()` |
| Password hashing with argon2id, limits on sign-in attempts | stolen password databases being cracked, passwords being guessed | `accounts.Options{NoRateLimits: true}` |
| Request body limit (1 MiB) | memory exhaustion by large uploads | `kit.WithBodyLimit(n)` |

## Security headers

Every response carries:

| Header | Value |
|---|---|
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=(), interest-cohort=()` |
| `Cross-Origin-Opener-Policy` | `same-origin` |
| `X-Permitted-Cross-Domain-Policies` | `none` |

`plugins/secure` changes them: `secure.AllowFraming()` lets the app's own pages frame it, `secure.Header(name, value)` sets one or, with `""`, removes it, and `secure.HSTS(maxAge, includeSubdomains)` tells browsers to use HTTPS only. HSTS isn't on by default because browsers remember it: turn it on once the site is served over HTTPS for good.

## Content-Security-Policy

Every page is sent:

```
default-src 'self'; script-src 'self' 'unsafe-hashes' 'sha256-7dQw…' 'nonce-…';
style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; font-src 'self' data:;
connect-src 'self'; object-src 'none'; base-uri 'self'
```

- **Scripts:** only the app's own files run, and inline scripts carrying the request's nonce. Tern puts it on its own scripts and on inline scripts in `<svelte:head>`; use `%tern.nonce%` in `src/app.html` for yours. Inline scripts in a page's body don't get it: that's where `{@html}` puts content that may come from users.
- **Svelte's event handler:** server rendering puts `onload="this.__e=event"` and `onerror="this.__e=event"` on images and similar elements, so their events aren't lost before hydration. `'unsafe-hashes'` with that text's hash allows exactly it, and no other inline handler. Tern adds it, and the nonce, to a policy of the app's own too.
- **WebAssembly:** an app with Go components that run in the browser also gets `'wasm-unsafe-eval'`, which lets the browser compile WebAssembly; JavaScript's `eval` stays blocked.
- **Styles** may be inline, as Svelte's transitions and `style:` directives need. Injected styles can't run code.
- **Images** may come from any HTTPS origin, for avatars and pictures hosted elsewhere.

To use something from another origin, name it:

```go
app, err := ternApp(
	kit.WithCSPSources("script-src", "https://plausible.io"),
	kit.WithCSPSources("connect-src", "https://plausible.io"),
	kit.WithCSPSources("frame-src", "https://www.youtube-nocookie.com"),
)
```

Under `tern dev`, the overlay shows each thing the policy blocked, with the line that allows it. `kit.WithContentSecurityPolicy(policy)` replaces the policy, `kit.WithContentSecurityPolicyReportOnly(policy)` tries one without enforcing it, and Tern adds the nonce to either. Prerendered pages are static files, sent without a policy.

## Rate limits

Form actions and other state-changing requests (POST, PUT, PATCH, DELETE) are limited to 120 a minute per client, in bursts of up to 60: more than a person sends, fewer than a script does. A client over the limit gets `429 Too Many Requests` with `Retry-After`. Reading pages isn't limited.

Clients are told apart by their IP address, from `kit.ClientIP`. Behind a reverse proxy, name it with `kit.WithTrustedProxies`, or every client has the proxy's address and shares one limit; the app logs a warning when requests carry `X-Forwarded-For` and no proxy is trusted. Clients on loopback addresses, such as `tern dev` and a proxy on the same machine, aren't limited.

To change the limit, add `plugins/ratelimit`, which replaces it:

```go
ratelimit.New(ratelimit.Rate(30, time.Minute), ratelimit.Methods()) // every request, 30 a minute
```

`accounts` has limits of its own on sign-in, sign-up and password reset attempts.

## Dependencies

`tern doctor` checks them before a deploy ([docs/deployment.md](deployment.md#before-you-deploy-tern-doctor)):

- **Go:** govulncheck, built into Tern, looks for known vulnerabilities in the Go modules and in the Go version itself, and fails the check for those the app's code reaches. Most are in Go's standard library, and a newer Go patch release fixes them.
- **npm:** `npm audit` or `pnpm audit`, whichever lockfile the project has. The packages end up in the browser; high and critical advisories are warnings.

Both need the network; `tern doctor -offline` skips them. To run them in CI, add `go tool tern doctor` as a step: it exits 1 when a check fails.

## Who may use a route

Every route with server code declares who may use it, and the build stops at one that doesn't:

```go
var Access = kit.SignedIn // or kit.Public, or kit.Permission("links.edit")
```

`kit.SignedIn` and `kit.Permission` cover the directories below; `kit.Public` covers its own only, so a route added later starts closed. Tern applies the rule to the page, its data, its form actions and its endpoints, before any of the route's code runs, and never serves a signed-in route from the page cache. `acc.Guard()` and `policy.Guard()` are the checks behind the rules; see the [guide](guide.md#who-may-use-a-route). `go tool tern routes` shows each route's rule, to review them in one place.

Rules say who may use a route, not which records a user may see: a load still asks for the user's own data (`shelf.List(ctx, user.ID, …)`), and returns 404 for someone else's.

## Migrations

`go tool tern migrate check` finds migrations that fail on a table with rows or break the version of the app still running during a deploy, and `tern doctor` runs it before each deploy; see [database migrations](guide.md#database-migrations).
