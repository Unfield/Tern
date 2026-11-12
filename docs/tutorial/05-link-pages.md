# 5. A page for each link

Each link gets a page of its own, at `/links/1`, `/links/2` and so on, where you open it, mark it read, or delete it.

## The store

Add the functions the page needs to `internal/shelf/shelf.go`:

```go
// Get returns one link.
func Get(ctx context.Context, id int64) (Link, error) {
	var l Link
	err := DB.QueryRowContext(ctx, `SELECT id, url, title, read, added_at FROM links WHERE id = ?`, id).
		Scan(&l.ID, &l.URL, &l.Title, &l.Read, &l.AddedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	return l, err
}

// SetRead marks a link read or unread.
func SetRead(ctx context.Context, id int64, read bool) error {
	return changed(DB.ExecContext(ctx, `UPDATE links SET read = ? WHERE id = ?`, read, id))
}

// Delete removes a link.
func Delete(ctx context.Context, id int64) error {
	return changed(DB.ExecContext(ctx, `DELETE FROM links WHERE id = ?`, id))
}

// changed turns an update that matched no row into ErrNotFound.
func changed(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return ErrNotFound
	}
	return nil
}
```

## A route with a parameter

Go import paths can't have brackets in them, so where SvelteKit writes `[id]`, Tern names the directory `id.param`. Add `.integer` and only numbers match: `/links/abc` is a 404 without your code running.

Create `src/routes/links/id.param.integer/page.server.go`:

```go
package link

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Unfield/Tern/kit"

	"shelf/internal/shelf"
)

type Data struct {
	Link shelf.Link `json:"link"`
}

func Load(ctx *kit.Ctx) (Data, error) {
	link, err := shelf.Get(ctx, id(ctx))
	if errors.Is(err, shelf.ErrNotFound) {
		return Data{}, kit.NotFound()
	}
	if err != nil {
		return Data{}, err
	}
	return Data{Link: link}, nil
}

// id is the link's ID from the URL. The integer matcher only lets
// numbers through, so it always parses.
func id(ctx *kit.Ctx) int64 {
	n, _ := strconv.ParseInt(ctx.Param("id"), 10, 64)
	return n
}

type Toggle struct {
	Read bool `form:"read"`
}

var Actions = kit.Actions{
	"read": kit.FormAction(func(ctx *kit.Ctx, in Toggle) (struct{}, error) {
		err := shelf.SetRead(ctx, id(ctx), in.Read)
		if errors.Is(err, shelf.ErrNotFound) {
			return struct{}{}, kit.NotFound()
		}
		return struct{}{}, err
	}),
	"delete": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		err := shelf.Delete(ctx, id(ctx))
		if errors.Is(err, shelf.ErrNotFound) {
			return struct{}{}, kit.NotFound()
		}
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/")
	}),
}
```

- **Errors are responses.** `kit.NotFound()` answers 404 and shows `src/routes/+error.svelte`. `kit.Redirect` sends the browser elsewhere; after a form post, use `303 See Other`. Any other error is logged, and the visitor sees a 500 page; the details are shown only under the dev server.
- **`kit.Action`** is an action without a form struct, for posts that carry no fields, like this delete.

Then the page, `src/routes/links/id.param.integer/+page.svelte`:

```svelte
<script lang="ts">
	import { enhance } from '@ternkit/tern';
	import { Badge, Button } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const link = $derived(data.link);
</script>

<svelte:head>
	<title>{link.title} · Shelf</title>
</svelte:head>

<p class="back"><a href="/">← Your shelf</a></p>

<h1>{link.title}</h1>
<p class="meta">
	{new URL(link.url).host} · saved {new Date(link.addedAt).toLocaleDateString()}
	{#if link.read}<Badge tone="success">Read</Badge>{/if}
</p>

<div class="actions">
	<Button href={link.url} target="_blank" rel="noopener noreferrer">Open</Button>
	<form method="POST" action="?/read" use:enhance>
		<input type="hidden" name="read" value={link.read ? '' : 'on'} />
		<Button type="submit" variant="secondary">{link.read ? 'Mark as unread' : 'Mark as read'}</Button>
	</form>
	<form method="POST" action="?/delete" use:enhance>
		<Button type="submit" variant="ghost">Delete</Button>
	</form>
</div>

<style>
	.back a {
		color: var(--tern-muted);
		text-decoration: none;
		font-size: 0.875rem;
	}
	h1 {
		margin: 1rem 0 0.25rem;
		font-size: 1.75rem;
		overflow-wrap: anywhere;
	}
	.meta {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin: 0 0 1.5rem;
		color: var(--tern-muted);
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}
</style>
```

These are plain HTML forms. `use:enhance` makes them post in the background and run the page's load again, so the button changes to "Mark as unread" in place. The hidden `read` field posts the state to switch to: a bool field is true when it posts `on`, as a checked checkbox does.

## Typed links

Now link the list to these pages. In `src/routes/+page.svelte`, import `route`:

```svelte
import { route } from '$routes';
```

And make each title link to its page:

```svelte
<a href={route('/links/[id]', { id: link.id })}>{link.title}</a>
```

`route` takes a route's pattern, which TypeScript checks: if you rename the `links` directory, `npm run check` points at every link that's now wrong. Patterns leave out matchers, so it's `[id]`, not `[id=integer]`.

Click a link, mark it read, then delete it: you're back on the shelf, and it's gone. Try `/links/999` too, for the not-found page.

Next: [Filtering](06-filters.md).
