# Security

Tern turns protections on without being asked, and turning one off takes an option with its name on it ([ADR 0027](adr/0027-secure-by-default.md)). `tern doctor` lists the ones an app has turned off, and what each costs.

## On by default

| Protection | What it stops | Turned off with |
|---|---|---|
| Cross-origin request protection | other sites posting forms to the app, or calling its endpoints with the user's cookies | `kit.WithoutCSRFProtection()` |
| Cookies `SameSite=Lax` and `Secure`, and session cookies `HttpOnly` | cookies sent with cross-site requests or over plain HTTP, and scripts reading sessions | — |
| Security headers | other sites framing the app (clickjacking), full URLs leaking in referrers, pages using the camera, microphone or location | `kit.WithoutSecurityHeaders()` |
| Content-Security-Policy | injected markup running scripts (cross-site scripting) | `kit.WithoutContentSecurityPolicy()` |
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
default-src 'self'; script-src 'self' 'nonce-…'; style-src 'self' 'unsafe-inline';
img-src 'self' data: blob: https:; font-src 'self' data:; connect-src 'self';
object-src 'none'; base-uri 'self'
```

- **Scripts:** only the app's own files run, and inline scripts carrying the request's nonce. Tern puts it on its own scripts and on inline scripts in `<svelte:head>`; use `%tern.nonce%` in `src/app.html` for yours. Inline scripts in a page's body don't get it: that's where `{@html}` puts content that may come from users.
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
