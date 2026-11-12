# 6. Filtering

A reading list is mostly about what's left to read. The shelf should show the unread links first, with the read ones a click away.

The filter goes in the URL: `/?show=read`. That way it survives a reload, works with the back button, and can be shared as a link, and the load reads it like any other part of the request.

## The load

In `src/routes/page.server.go`, read `show` from the query and pass it to `List`, which already takes it:

```go
type Data struct {
	Links []shelf.Link `json:"links"`
	Show  string       `json:"show"`
}

func Load(ctx *kit.Ctx) (Data, error) {
	show := ctx.URL().Query().Get("show")
	if show != "read" && show != "all" {
		show = "unread"
	}
	links, err := shelf.List(ctx, show)
	if err != nil {
		return Data{}, err
	}
	return Data{Links: links, Show: show}, nil
}
```

Anything other than `read` or `all` means unread, so a mistyped URL still shows something sensible.

## The page

In `src/routes/+page.svelte`, list the filters in the script:

```ts
const filters = [
	{ show: 'unread', label: 'To read' },
	{ show: 'read', label: 'Read' },
	{ show: 'all', label: 'All' }
];
```

Put them above the list, and say what's missing when a filter is empty:

```svelte
<nav class="filters" aria-label="Show">
	{#each filters as f (f.show)}
		<a href="?show={f.show}" aria-current={data.show === f.show ? 'page' : undefined}>{f.label}</a>
	{/each}
</nav>

{#if data.links.length === 0}
	<p class="empty">{data.show === 'read' ? 'Nothing read yet.' : 'Nothing here. Save a link above.'}</p>
{:else}
```

And style them:

```css
.filters {
	display: flex;
	gap: 0.25rem;
	margin-bottom: 0.5rem;
}
.filters a {
	padding: 0.25rem 0.75rem;
	border-radius: 999px;
	color: var(--tern-muted);
	font-size: 0.875rem;
	text-decoration: none;
}
.filters a[aria-current='page'] {
	background: var(--tern-surface);
	color: var(--tern-fg);
}
```

The filters are plain links. When you click one, the router sees that it's the same page with a different query, asks the server for the page's data only, and updates the list. No code of yours handles the click. Mark a link read on its page, go back, and it has moved from "To read" to "Read".

Next: [Accounts](07-accounts.md).
