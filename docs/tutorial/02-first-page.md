# 2. Your first page

Shelf's home page lists the saved links. Before there's a database, start with a few links in Go, to see how data gets from the server to the page.

## The data

The links need a type the whole app can use, so give them a package of their own. Create `internal/shelf/shelf.go`:

```go
// Package shelf keeps the links.
package shelf

import "time"

// Link is a page saved to read later.
type Link struct {
	ID      int64     `json:"id"`
	URL     string    `json:"url"`
	Title   string    `json:"title"`
	Read    bool      `json:"read"`
	AddedAt time.Time `json:"addedAt"`
}
```

The `json` tags name the fields as the page sees them.

## The load

Replace `src/routes/page.server.go` with a `Load` that returns links:

```go
package routes

import (
	"time"

	"github.com/Unfield/Tern/kit"

	"shelf/internal/shelf"
)

type Data struct {
	Links []shelf.Link `json:"links"`
}

func Load(ctx *kit.Ctx) (Data, error) {
	return Data{Links: []shelf.Link{
		{ID: 2, URL: "https://svelte.dev/blog/runes", Title: "Introducing runes", AddedAt: time.Now()},
		{ID: 1, URL: "https://go.dev/blog/go1.26", Title: "Go 1.26 is released", Read: true, AddedAt: time.Now()},
	}}, nil
}
```

`Load` runs on the server for every request to the page, and again when the browser navigates to it. Whatever it returns becomes the page's `data`.

## The page

Replace `src/routes/+page.svelte`:

```svelte
<script lang="ts">
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>Shelf</title>
</svelte:head>

<h1>Your shelf</h1>

{#if data.links.length === 0}
	<p class="empty">Nothing saved yet.</p>
{:else}
	<ul class="links">
		{#each data.links as link (link.id)}
			<li class:read={link.read}>
				<a href={link.url}>{link.title}</a>
				<span class="host">{new URL(link.url).host}</span>
			</li>
		{/each}
	</ul>
{/if}

<style>
	h1 {
		margin: 0 0 1.5rem;
		font-size: 1.75rem;
	}
	.empty {
		color: var(--tern-muted);
	}
	.links {
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.links li {
		display: flex;
		align-items: baseline;
		gap: 0.75rem;
		padding: 0.75rem 0;
		border-bottom: 1px solid var(--tern-border);
	}
	.links a {
		color: var(--tern-fg);
		font-weight: 500;
		text-decoration: none;
	}
	.links a:hover {
		text-decoration: underline;
	}
	.read a {
		color: var(--tern-muted);
	}
	.host {
		color: var(--tern-muted);
		font-size: 0.875rem;
	}
</style>
```

The two links appear, the one already read in a lighter color. The `--tern-*` colors come from Tern UI's theme, so the page follows the system's light or dark mode.

The layout still links to the About page you deleted. In `src/routes/+layout.svelte`, remove it from the `links` list, leaving Home; [step 7](07-accounts.md) gives the layout a new header.

## Types from Go

Hover `data.links` in your editor: it's a `Link[]`, with `id: number`, `url: string`, `read: boolean` and `addedAt: string`. Tern generated that type from the Go struct, through the `./$types` import. Rename a field in Go, and `npm run check` reports every place in your Svelte code that still uses the old name. `time.Time` arrives as a string, as JSON sends it, so the page turns it into a date with `new Date(link.addedAt)`.

The page was rendered on the server, in Go: view the page's source and the links are in the HTML. Then the browser takes over, and later navigations only fetch the data.

Next: [A database](03-database.md).
