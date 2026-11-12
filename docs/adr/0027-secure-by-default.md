# 0027. Secure by default

- Status: Accepted
- Date: 2026-11-12

## Context

Tern already turns some protections on without being asked: cross-origin form posts are refused, cookies are `SameSite=Lax` and `Secure`, passwords are hashed with argon2id, and sign-in attempts are limited. Others wait for the developer to remember them:

- **Security headers** come from `plugins/secure`, which only the SaaS template and three examples add.
- **A Content-Security-Policy** is one option away (`kit.WithContentSecurityPolicy`, with nonces on Tern's own scripts), but no app gets one unless it writes it.
- **Request rates** are limited by `plugins/ratelimit`, which only the analytics example uses.
- **Access to routes** is checked in each `Load` and action, with `acc.Require` or `policy.Require`. A route that forgets the call is open to everyone, and nothing notices.

The last is the most common way real apps leak data, and the hardest to see in review: the bug is a missing line.

## Decision

From v1.16, an app made with `kit.New` gets each of these without code, and turning one off takes an option with its name on it.

**Security headers.** `kit` sends the headers `plugins/secure` sent: `X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`, a `Permissions-Policy` that turns off the camera, microphone and location, `Cross-Origin-Opener-Policy: same-origin` and `X-Permitted-Cross-Domain-Policies: none`, besides the `X-Content-Type-Options: nosniff` it already sent. `plugins/secure` stays, to change them (`AllowFraming`, `Header`, `HSTS`); `kit.WithoutSecurityHeaders()` drops them. HSTS stays opt-in, since it can't be taken back once browsers have seen it.

**A Content-Security-Policy.** Pages get

```
default-src 'self'; script-src 'self' 'unsafe-hashes' 'sha256-7dQw…' 'nonce-…';
style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; font-src 'self' data:;
connect-src 'self'; object-src 'none'; base-uri 'self'
```

- **Scripts** are the point: only the app's own files and inline scripts with the request's nonce run, so markup injected into a page can't run code.
- **Inline scripts in `<svelte:head>`**, such as mode-watcher's theme script, get the nonce: the head is the app's own markup. The body's don't, since that's where `{@html}` puts content that may come from users.
- **Svelte's inline event handler**, `onload="this.__e=event"` and `onerror="this.__e=event"`, which server rendering puts on elements whose events must survive until hydration, is allowed by its hash with `'unsafe-hashes'`: that text, and no other handler. Tern adds it to any policy, as it adds the nonce.
- **WebAssembly:** apps with Go components that run in the browser get `'wasm-unsafe-eval'`, which allows compiling WebAssembly and not JavaScript's `eval`.
- **Styles** allow `'unsafe-inline'`: Svelte's transitions and `style:` directives set inline styles, and injected styles can't run code.
- **Images** from any HTTPS origin are allowed, as apps show avatars and pictures from elsewhere.
- **`tern dev`** adds Vite's origin and its WebSocket, so the policy is the same in development, where a page that breaks under it breaks first.
- `kit.WithCSPSources(directive, sources...)` adds sources, for an analytics script or an embed; `kit.WithContentSecurityPolicy` replaces the policy; `kit.WithoutContentSecurityPolicy()` sends none.

`form-action` and `frame-ancestors` are left out: browsers apply `form-action` to the redirects after a form post, which would break sign-in with OAuth and checkout with Stripe, and `X-Frame-Options` already forbids framing.

**Rate limits.** State-changing requests (POST, PUT, PATCH, DELETE) are limited to 120 a minute per client, in bursts of up to 60, which no person reaches and a script does. Adding `plugins/ratelimit` replaces the default with its own settings; `kit.WithoutRateLimit()` removes it. Clients are told apart by `kit.ClientIP`. Behind a proxy that isn't trusted, every client would share the proxy's address, so the app logs a warning, once, when a request carries `X-Forwarded-For` from a peer `kit.WithTrustedProxies` doesn't name.

**Routes closed by default.** A route's server code says who may use it, with a package variable that code generation reads:

```go
var Access = kit.Public               // anyone
var Access = kit.SignedIn             // a signed-in user
var Access = kit.Permission("links.edit") // a signed-in user with the permission
```

- **Inheritance:** a layout's `Access` applies to the routes under it, and a route's own replaces it, so a `(app)` group can be signed-in and its login page public.
- **Closed:** a route with server code (a load, actions, or an endpoint) that has no `Access`, of its own or from a layout, fails code generation with the route's file and the line to add. A page without server code serves nothing the client doesn't already have, and needs none.
- **Enforced by `kit`** before the loads, actions and endpoints run, for pages, their `__data.json` and form posts alike.
- **Pluggable:** `kit` doesn't know about users. `kit.WithSignedIn(fn)` and `kit.WithPermissions(fn)` give it the checks; `acc.Guard()` and `policy.Guard()` are those for `accounts` and `rbac`. An app whose routes ask for `SignedIn` without a check configured doesn't start.
- **Prerendered** routes must be `kit.Public`.

The calls to `acc.Require` in loads keep working, to get the user. They're no longer what keeps others out.

## Consequences

- **Upgrading breaks builds** of apps with server routes and no `Access`, on purpose. `tern fix access` adds `var Access = kit.Public` where a route has none, which keeps what the app did, for the developer to tighten route by route; the error message names it. The other defaults change what a running app sends, and the CHANGELOG lists each with the option that restores the old behaviour.
- **An app with third-party scripts** (analytics, chat widgets, embeds) has them blocked until it names their origins with `kit.WithCSPSources`. The browser's console says which, and so does the dev overlay.
- **`tern doctor`** checks for the opt-outs and says what each costs, instead of checking for `plugins/secure`.
- Tern's own templates and examples are brought under the defaults, which tests them.
