# 1. Create the app

Create the project:

```sh
npm create tern@latest
```

It asks a few questions. Answer them like this:

- **Where should the project go?** `shelf`
- **Start with a bare app or a full one?** `bare`. The full app has accounts, teams and billing already built, which is a good start for a real product but would skip what this tutorial is about.
- **Which template?** `minimal`
- **Style it with Tern UI?** Yes. [Tern UI](../ui.md) is Tern's component library: buttons, forms, sign-in screens and more. Using it keeps this tutorial on the parts that matter.
- **Create a git repository?** and **Run npm install?** Yes

Without npm, `go run github.com/Unfield/Tern/cmd/tern@latest new -template minimal -ui -install shelf` does the same.

## Start the dev server

```sh
cd shelf
npm run dev
```

Open <http://localhost:5173>. You see a page that says "Hello from Go!", and a form that greets you.

`npm run dev` runs `go tool tern dev`. The Tern CLI is pinned in your `go.mod`, so everyone on the project uses the same version. The dev server reloads Svelte components in place as you edit them, and rebuilds and restarts the Go server when Go code changes.

## What's in the project

```
shelf/
├── main.go                  starts the server
├── main_test.go             tests of the whole app
├── src/
│   ├── app.html             the HTML around every page
│   └── routes/
│       ├── +layout.svelte   the frame around every page
│       ├── +page.svelte     the page at /
│       ├── page.server.go   its Go code: the data it loads, and its form
│       ├── +error.svelte    shown when a page fails
│       └── about/
│           └── +page.svelte the page at /about
├── static/                  files served as they are
├── go.mod
└── package.json
```

Every directory under `src/routes` is a URL. A `+page.svelte` makes it a page, and the `.go` files next to it are its server code, an ordinary Go package. That's the whole model: pages are Svelte, the server is Go, and they live side by side.

Open `src/routes/page.server.go`:

```go
// Data is passed to +page.svelte. Its TypeScript type is generated for you.
type Data struct {
	Message    string    `json:"message"`
	ServerTime time.Time `json:"serverTime"`
}

// Load runs on the server for every request to this page.
func Load(ctx *kit.Ctx) (Data, error) {
	return Data{
		Message:    "Hello from Go!",
		ServerTime: time.Now(),
	}, nil
}
```

And `src/routes/+page.svelte` uses it:

```svelte
<script lang="ts">
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
</script>

<h1>{data.message}</h1>
```

Change the message in Go and save. The dev server restarts the server and the page shows the new message.

Shelf won't need the About page. Delete `src/routes/about`, and the dev server drops its route.

Next: [Your first page](02-first-page.md).
