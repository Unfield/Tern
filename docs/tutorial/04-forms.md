# 4. Saving links

Links get onto the shelf through a form. In Tern, a form posts to an **action**: a Go function in the page's package that handles the post.

## Saving to the database

Add a function to `internal/shelf/shelf.go` that inserts a link:

```go
// Add saves a link. Without a title, the address is the title.
func Add(ctx context.Context, url, title string) (Link, error) {
	if title == "" {
		title = url
	}
	l := Link{URL: url, Title: title, AddedAt: time.Now().UTC()}
	err := DB.QueryRowContext(ctx, `INSERT INTO links (url, title, added_at) VALUES (?, ?, ?) RETURNING id`, l.URL, l.Title, l.AddedAt).Scan(&l.ID)
	return l, err
}
```

## The action

Add the form and its action to `src/routes/page.server.go`, below `Load`:

```go
// NewLink is the form that adds a link. Its tags are the rules: Tern
// checks them on the server, and in the browser as the user types.
type NewLink struct {
	URL   string `form:"url" validate:"required,url,max=2000" label:"Address"`
	Title string `form:"title" validate:"max=200"`
}

type Added struct {
	Link shelf.Link `json:"link"`
}

var Actions = kit.Actions{
	"add": kit.FormAction(func(ctx *kit.Ctx, in NewLink) (Added, error) {
		link, err := shelf.Add(ctx, in.URL, strings.TrimSpace(in.Title))
		if err != nil {
			return Added{}, err
		}
		return Added{Link: link}, nil
	}),
}
```

Add `"strings"` to the imports.

`kit.FormAction` decodes the posted form into a `NewLink` and checks its `validate` rules before your function runs. Here the address is required, must be an `http` or `https` address, and is at most 2,000 characters; `label` names the field in messages. If a rule fails, the action answers 400 with the errors, and your function isn't called. [Validating forms](../guide.md#validating-forms) lists every rule.

## The form

In `src/routes/+page.svelte`, import Tern UI's form components and the rules, and take the `form` prop:

```svelte
<script lang="ts">
	import { Button, Field, Form, Input } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { data, form }: PageProps = $props();
	let saving = $state(false);
</script>
```

Then put the form under the heading:

```svelte
<h1>Your shelf</h1>

<Form action="?/add" rules={forms.add} {form} bind:submitting={saving} class="add">
	<Field label="Address" name="url"><Input type="url" placeholder="https://" /></Field>
	<Field label="Title" name="title"><Input placeholder="Optional" /></Field>
	<Button type="submit" loading={saving}>Save</Button>
</Form>
```

And lay it out in a row, in the `<style>`:

```css
:global(.add) {
	display: grid;
	grid-template-columns: 2fr 1fr auto;
	gap: 0.75rem;
	align-items: start;
	margin-bottom: 2rem;
}
:global(.add) > :global(button) {
	margin-top: 1.5rem;
}
@media (max-width: 40rem) {
	:global(.add) {
		grid-template-columns: 1fr;
	}
	:global(.add) > :global(button) {
		margin-top: 0;
	}
}
```

Type something that isn't an address and the field says so, before anything is posted: `./$forms` holds the same rules as the Go struct, generated from it, and `Form` checks them as you type. Save a real address and it appears in the list, and the form clears.

Here's what happened:

- `action="?/add"` posts to the `add` action of this page.
- `Form` posts in the background. When the action succeeds, it clears the form and runs the page's `Load` again, so the list shows the new link without a reload.
- The server checks the rules again, whatever the browser did. Without JavaScript the form still works: the browser posts it, and Tern renders the page again with the errors or the result in `form`.
- `form` is the action's result, or its errors, typed from the Go code like `data` is.

Next: [A page for each link](05-link-pages.md).
