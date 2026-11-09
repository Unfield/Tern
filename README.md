<h1 align="center">Tern</h1>

<p align="center">
  <strong>Svelte on the client. Go on the server.</strong><br>
  A full-stack web framework with file-based routing, typed data loading and single-binary deploys.
</p>

<p align="center">
  <a href="https://github.com/Unfield/Tern/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/Unfield/Tern/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://pkg.go.dev/github.com/Unfield/Tern"><img alt="Go Reference" src="https://pkg.go.dev/badge/github.com/Unfield/Tern.svg"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
</p>

> [!WARNING]
> Tern is in early development and is not ready for production use. APIs will change before v1.0.

---

## Why Tern?

[SvelteKit](https://svelte.dev/docs/kit) has one of the best developer experiences on the web, but its server code runs on Node. Tern keeps the parts that make SvelteKit pleasant, such as file-based routes, `load` functions, form actions and layouts, and runs everything server-side in **Go**.

- **Real Svelte 5.** Not a lookalike. Your components, the Svelte ecosystem and your editor tooling all keep working.
- **Go on the server.** `load` functions and form actions are plain Go, built on `net/http`.
- **End-to-end types.** The structs your Go `Load` returns become TypeScript types for your Svelte pages, generated automatically.
- **One binary.** `tern build` embeds the compiled client into a single static Go executable. No Node in production.
- **Progressive enhancement.** Forms work without JavaScript, and get better with it.

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

> The snippets above show the target API; it is being built now. See the [roadmap](docs/ROADMAP.md).

## Status

| Milestone | Scope | State |
|---|---|---|
| **v0.1** | Router, `load`, actions, typegen, dev server, client rendering, single-binary build | In progress |
| **v0.2** | Server-side rendering and hydration | Planned |
| **v0.3** | Streaming, prerendering, adapters | Planned |

## Documentation

- [Architecture](docs/ARCHITECTURE.md): how Tern works internally
- [Roadmap](docs/ROADMAP.md): what is planned, and in what order
- [Decision records](docs/adr): why things are the way they are

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) and our [Code of Conduct](CODE_OF_CONDUCT.md) first. To report a security issue, follow [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE) © The Tern Authors

Tern is an independent project and is not affiliated with or endorsed by the Svelte team.
