# Testing

Tern apps are tested with Go's own `go test`, and in the browser with Playwright. Projects created with `tern new` come with both: `npm test` runs the Go tests, and `npm run test:e2e` runs the browser tests against the production build.

## Loads and actions

`kit/kittest` makes the request a load, action or endpoint gets, so you can call it directly and check what it returns:

```go
func TestPost(t *testing.T) {
	app := kittest.App(t)
	c := kittest.Ctx(t, app,
		kittest.Path("/blog/hello"),
		kittest.Route("/blog/slug.param", "slug", "hello"),
		kittest.Parent(routes.Root{User: "ada"}), // what kit.Parent decodes
	)
	post, err := blog.Load(c)
	if err != nil || post.Title != "Hello" {
		t.Fatal(post, err)
	}
}
```

The options describe the request:

| Option | |
|---|---|
| `Path("/a?b=1")`, `Method("PUT")` | the URL and method, `GET /` by default |
| `Form(url.Values{…})`, `JSON(v)` | a form for an action, or a JSON body for an endpoint; both make it a `POST` |
| `Header(k, v)`, `Cookie(c)` | headers and cookies |
| `Route(id, "name", "value", …)` | the route's ID and parameters |
| `Parent(v)` | the layouts' data |
| `Local(k, v)` | values in `Locals` |
| `NoHooks()` | skip the app's hooks, which run by default as they do before a request's loads |

Errors that answer a request have helpers: `kittest.Redirected(err)` returns a redirect's status and location, `kittest.Failed(err)` an action's `Fail` data, and `kittest.Status(err)` the status any error responds with. `kittest.Response(c)` is what the Ctx wrote so far, with the cookies and headers it set.

`kittest.App(t, opts…)` is an app with no routes, for calling code with a Ctx. Pass it the options your real app uses, its plugins among them, so their hooks run.

## Signed in

With `accounts`, `acc.ActAs(c, user)` makes a user the signed-in one for the rest of the request, without a session:

```go
db := kittest.SQLite(t, migrations.FS)
acc, _ := accounts.New(db, accounts.Options{})
_ = acc.Migrate(context.Background())
app := kittest.App(t, kit.WithPlugins(acc))

c := kittest.Ctx(t, app, kittest.Path("/projects"))
acc.ActAs(c, &accounts.User{ID: 1, Email: "ada@example.com"})
data, err := projects.Load(c)
```

## A database for each test

`kittest.SQLite(t, migrations…)` is an empty SQLite database in memory, with the migrations applied (see [migrations](guide.md#database-migrations)). Every call gets its own, and it's closed when the test ends, so tests can run in parallel and never see each other's rows.

## The whole app

In package `main`, next to the generated `ternApp`, `kittest.NewClient` serves the real app and requests its pages as a browser would. It keeps cookies, sends forms from the app's own origin, and doesn't follow redirects, so tests can check them. The app is started first, so its migrations run.

```go
func TestSignUp(t *testing.T) {
	app, err := ternApp()
	if err != nil {
		t.Fatal(err)
	}
	client := kittest.NewClient(t, app)
	page := client.PostForm("/signup", url.Values{"email": {"ada@example.com"}, "password": {"a long password"}})
	if page.Status != 303 || page.Location() != "/dashboard" {
		t.Fatal(page.Status, page.Location())
	}
	page = client.Get("/dashboard")
	if page.Text("h1") != "Welcome, Ada" || !page.Has("nav a[href=/settings]") {
		t.Error("the dashboard")
	}
}
```

`page.Find`, `page.First`, `page.Has` and `page.Text` query the server-rendered HTML with CSS selectors:
- tag names, `#id`, `.class` and `[attr]` or `[attr=value]`, combined, as in `a.button[href]`;
- descendant (`main a`) and child (`ul > li`) combinators;
- lists (`h1, h2`).

`kittest.Ctx` and `NewClient` work with `go test`'s `-race` and `t.Parallel()`.

## In the browser

`npm run test:e2e` runs Playwright against the production build: `playwright.config.ts` builds the app, starts it on port 4173 and runs `tests/*.spec.ts`. The first run needs a browser: `npx playwright install chromium`. The template's test loads the home page and fails on any error in the browser's console.
