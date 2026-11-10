# Coming from SvelteKit

Tern keeps SvelteKit's model of file-based routes, loads, form actions, layouts and progressive enhancement. Your Svelte components move over almost unchanged. What changes is the server side: loads, actions, hooks and endpoints are Go, and the app ships as one Go binary with no Node in production.

This page maps what you know to Tern. The [guide](guide.md) has the details.

## Files and routes

| SvelteKit | Tern |
|---|---|
| `src/routes/+page.svelte`, `+layout.svelte`, `+error.svelte` | the same |
| `+page.server.ts` with `load` | any `.go` file in the directory, with `func Load(ctx *kit.Ctx) (Data, error)` |
| `+layout.server.ts` with `load` | `func LayoutLoad(ctx *kit.Ctx) (Data, error)` |
| `+page.ts` / `+layout.ts` (universal load) | not needed: data comes from Go; fetch in `onMount` for client-only data |
| `+server.ts` with `GET`, `POST`, … | `func GET(ctx *kit.Ctx) error`, or a [typed endpoint](guide.md#typed-endpoints) `func GET(ctx *kit.Ctx, in In) (Out, error)` |
| `[slug]` | `slug.param` |
| `[[lang]]` | `lang.optional` |
| `[...path]` | `path.rest` |
| `(group)` | `app.group` |
| `[id=integer]` (matcher) | `id.param.integer`, with matchers registered by `kit.WithMatcher` |
| `+page.md` with mdsvex | `+page.md`, built in; collections with the `content` package |

Directory names change because Go import paths can't contain brackets or parentheses. Links don't change: `route('/blog/[slug]', { slug })` from `$routes` builds `/blog/hello`, and TypeScript checks the pattern exists.

## Loading data

```ts
// SvelteKit: +page.server.ts
export const load = async ({ params, locals }) => {
	const post = await db.post(params.slug);
	if (!post) error(404, 'Not found');
	return { post };
};
```

```go
// Tern: page.server.go in the same directory
func Load(ctx *kit.Ctx) (Page, error) {
	post, err := db.Post(ctx, ctx.Param("slug"))
	if errors.Is(err, sql.ErrNoRows) {
		return Page{}, kit.NotFound()
	}
	return Page{Post: post}, err
}
```

- **Types:** the returned struct's TypeScript type is generated. `PageProps` from `./$types` works as in SvelteKit.
- **Parent data:** `await parent()` becomes `kit.Parent[LayoutData](ctx)`.
- **Errors and redirects:** `error(404, …)` becomes `kit.NotFound()` or `kit.Error(status, message)`. `redirect(303, '/login')` becomes `kit.Redirect(http.StatusSeeOther, "/login")`.
- **Streaming:** promises in returned data become `kit.Defer(func() (T, error) { … })`.
- **Reruns:** `depends('app:x')` and `invalidate('app:x')` become `ctx.Depends("app:x")` and `invalidate('app:x')`. Layout loads rerun only when what they read changes, as in SvelteKit.

## Form actions

```go
var Actions = kit.Actions{
	"default": kit.Action(func(ctx *kit.Ctx) (Saved, error) {
		form, err := ctx.Form()
		if err != nil {
			return Saved{}, err
		}
		if form.Get("email") == "" {
			return Saved{}, kit.Fail(400, Invalid{Error: "Enter your email."}) // fail(400, …)
		}
		return Saved{OK: true}, nil
	}),
}
```

`use:enhance`, `form` props, named actions (`?/name`) and `applyAction` work as in SvelteKit.

## Hooks, locals and cookies

| SvelteKit | Tern |
|---|---|
| `handle` in `hooks.server.ts` | `kit.WithHook(func(ctx *kit.Ctx) error)`, or `kit.WithMiddleware` for `net/http` middleware |
| `event.locals` | `ctx.Locals` |
| `cookies.get` / `cookies.set` | `ctx.Cookie` / `ctx.SetCookie` |
| `setHeaders` | `ctx.SetHeader` |
| `handleError` | errors are logged with the request; customize `+error.svelte` |
| `hooks.client.ts` | `beforeNavigate`, `onNavigate` and `afterNavigate` from `@ternkit/tern` |

Sessions, CSRF protection (built in), security headers, rate limiting, CORS, metrics and health checks are [plugins](guide.md#plugins).

## Client modules

| SvelteKit | Tern |
|---|---|
| `$app/state`: `page`, `navigating` | `page`, `navigating` from `@ternkit/tern` |
| `$app/navigation`: `goto`, `invalidate`, `invalidateAll`, `preloadData`, `beforeNavigate`, `afterNavigate`, `onNavigate` | the same names from `@ternkit/tern` |
| `$app/forms`: `enhance`, `applyAction` | `@ternkit/tern` |
| `$env/static/public`, `$env/dynamic/public` | `env` from `@ternkit/tern`, with `PUBLIC_` variables from the server's environment |
| `$env/*/private` | `os.Getenv` in Go |
| `$app/paths`: `base`, `resolve` | `route()` from `$routes`, which includes the base path |
| `$service-worker` | the same |
| `$lib` | the same |

## Configuration and deployment

- **`svelte.config.js`:** no adapter. `vite.config.js` uses `tern()` from `@ternkit/tern/vite`, and the few settings Go and Vite share (base path, i18n, service worker) live in an optional `tern.json`.
- **Prerendering:** `export const prerender = true` becomes `const Prerender = true`. `entries()` becomes `func Entries(ctx *kit.Ctx) ([]map[string]string, error)`.
- **Turning off SSR:** `export const ssr = false` becomes `const SSR = false`.
- **Building:** `tern build` produces one static binary with the client, the server renderer and your Go code. Run it anywhere: a container, a VM or a systemd unit. See [deployment](deployment.md).

## Server rendering

Pages render on the server in a JavaScript engine embedded in the Go binary (QuickJS), not in Node:

- **Web APIs:** `URL`, `TextEncoder`, `crypto.randomUUID`, `structuredClone`, `AbortController` and the other APIs libraries commonly use are there. Popular libraries are [checked in CI](../tests/ssr-compat). Node built-ins (`fs`, `Buffer`) are not.
- **Data fetching:** `fetch` is not available while rendering, because data comes from Go loads.
- **`Intl`:** formats like `en-US` on the server. The browser re-renders with the user's locale.

## What Tern adds

These are built in, where SvelteKit leaves them to libraries:

- **Data and background work:** database migrations, background jobs and cron.
- **Realtime:** server-sent events and WebSockets.
- **Auth:** sign-in with OAuth providers, an OpenID Connect provider of your own, and roles.
- **Content and assets:**
  - email with Svelte templates
  - file uploads and storage, on disk or in S3
  - responsive images
  - translations
- **APIs:** OpenAPI for typed endpoints.

## What's different or missing

- **Universal loads:** no `+page.ts` or `+layout.ts`, since data comes from Go.
- **Adapters:** none. Tern always builds a Go binary. For static hosting, prerender every page.
- **Shallow routing:** `pushState` and `replaceState` from `$app/navigation` aren't available. `goto(url, { state })` and `page.state` cover the common case.
- **Snapshots:** not available.
