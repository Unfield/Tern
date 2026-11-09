<p align="center"><img src="docs/assets/logo.svg" alt="" width="88" height="88"></p>

<h1 align="center">Tern</h1>

<p align="center">
  <strong>Svelte on the client. Go on the server.</strong><br>
  A full-stack web framework with file-based routing, typed data loading and single-binary deploys.
</p>

<p align="center">
  <a href="https://github.com/Unfield/Tern/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/Unfield/Tern/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://pkg.go.dev/github.com/Unfield/Tern"><img alt="Go Reference" src="https://pkg.go.dev/badge/github.com/Unfield/Tern.svg"></a>
  <a href="https://unfield.github.io/Tern/"><img alt="Docs" src="https://img.shields.io/badge/docs-unfield.github.io%2FTern-0f766e"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
</p>

> [!NOTE]
> Tern 1.0 is stable: minor releases do not break the public API. See [the stability promise](docs/stability.md).

---

## Why Tern?

[SvelteKit](https://svelte.dev/docs/kit) has one of the best developer experiences on the web, but its server code runs on Node. Tern keeps the parts that make SvelteKit pleasant, such as file-based routes, `load` functions, form actions and layouts, and runs everything server-side in **Go**.

- **Real Svelte 5.** Not a lookalike. Your components, the Svelte ecosystem and your editor tooling all keep working.
- **Go on the server.** `load` functions and form actions are plain Go, built on `net/http`.
- **End-to-end types.** The structs your Go `Load` returns become TypeScript types for your Svelte pages, generated automatically.
- **Server-side rendering.** Pages render in Go through an embedded JavaScript engine and hydrate in the browser. No Node in production.
- **One binary.** `tern build` embeds the client and the renderer into a single static Go executable.
- **Progressive enhancement.** Forms work without JavaScript, and get better with it.

## Getting started

You need Go 1.26+ and Node.js 22+.

```sh
go run github.com/Unfield/Tern/cmd/tern@latest new my-app
cd my-app
npm install
npm run dev
```

Open <http://localhost:5173>. `npm run build` produces `build/my-app`, a single binary that serves the whole app.

## A taste

```
src/routes/
├── +layout.svelte
├── +page.svelte
└── blog/slug.param/        # → /blog/[slug]
    ├── page.server.go
    └── +page.svelte
```

```go
// src/routes/blog/slug.param/page.server.go
package page

import "github.com/Unfield/Tern/kit"

type Data struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func Load(ctx *kit.Ctx) (Data, error) {
	post, err := posts.Find(ctx.Param("slug"))
	if err != nil {
		return Data{}, kit.NotFound()
	}
	return Data{Title: post.Title, Body: post.Body}, nil
}
```

```svelte
<!-- src/routes/blog/slug.param/+page.svelte -->
<script lang="ts">
  import type { PageProps } from './$types';
  let { data }: PageProps = $props(); // fully typed from the Go struct
</script>

<h1>{data.title}</h1>
<article>{data.body}</article>
```

Dynamic directories use Go-safe names, because Go import paths cannot contain brackets: `slug.param` for `[slug]`, `lang.optional` for `[[lang]]`, `path.rest` for `[...path]`, and `app.group` for `(app)`. See [examples/todos](examples/todos) for a complete app with sessions, form actions, a protected route group and an API endpoint.

## Status

| Milestone | Scope | State |
|---|---|---|
| **v0.1** | Router, loads, actions, endpoints, typegen, dev server, single-binary build | Done |
| **v0.2** | Server-side rendering and hydration | Done |
| **v0.3** | Prerendering, streaming, deployment guides | Done |
| **v0.4** | Typed links, navigation hooks, selective reloading, CSP, a shadcn-svelte dashboard example, docs site | Done |
| **v1.0** | Plugins, base paths, public environment variables, the stability promise | Done |

## Documentation

- [Guide](docs/guide.md): everything you need to build an app
- [Architecture](docs/ARCHITECTURE.md): how Tern works internally
- [Deployment](docs/deployment.md): running Tern in production
- [Benchmarks](docs/benchmarks.md): how fast the pieces are
- [Stability](docs/stability.md): what the version numbers promise
- [Roadmap](docs/ROADMAP.md): what is planned, and in what order
- [Decision records](docs/adr): why things are the way they are

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) and our [Code of Conduct](CODE_OF_CONDUCT.md) first. To report a security issue, follow [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE) © The Tern Authors

Tern is an independent project and is not affiliated with or endorsed by the Svelte team.
