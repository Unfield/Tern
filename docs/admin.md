# Admin screens

The `admin` package gives an app screens to manage its own data, generated from Go structs: for each kind of record, a list with search, filters, sorting and pages, and a form to create, edit and delete one. The forms check the same `validate` rules as [form actions](guide.md#forms): in Go, and in the browser as the admin types.

## A resource

Describe a table with a struct. Each field is a column: its `db` tag, or else its name in snake_case (`AddedAt` is `added_at`):

```go
type Link struct {
	ID      int64     `admin:"key"`
	Title   string    `validate:"required,max=200" admin:"list,search"`
	URL     string    `validate:"required,url" label:"Address" admin:"list,search"`
	Read    bool      `admin:"list,filter"`
	UserID  int64     `label:"Owner" admin:"list,ref=users"`
	AddedAt time.Time `admin:"list,readonly"`
}
```

The `admin` tag says how a field is shown:

| Option | Means |
|---|---|
| `key` | The record's key. A field named `ID` is the key without it. A number is set by the database; text is entered when the record is created. |
| `list` | A column in the list. Without any, the list shows the first five fields. |
| `search` | The list's search box looks in it. Text only. |
| `filter` | A filter above the list: for checkboxes, and for fields with `oneof`. |
| `readonly` | Shown, never changed. When a record is created, the database fills it in, from the column's default. |
| `textarea` | Several lines of text. |
| `date` | A `time.Time` holds a day, not a moment. |
| `unixms`, `unix` | An integer holds a time, in milliseconds or seconds since 1970. It's shown and edited as one. |
| `ref=users` | It holds a key of another resource, and links to it. |
| `-` | Left out. |

Fields take their label from the `label` tag, or from their name, and help text from `description`. A pointer is a column that may be empty: an empty field saves `NULL`. A `validate:"oneof=a|b"` field is a menu, and a field whose name has "password" in it is a password field, which isn't shown in lists and stays as it is when left empty.

After the rules, a struct's `Validate() error` method runs. It can fail with `kit.FieldError`, as in a form action.

## The panel

Make a store for each table, and a panel with them:

```go
links, err := admin.NewTable[Link](db, "links", "")
if err != nil {
	return err
}
users, err := admin.NewTable[User](db, "tern_users", "")
if err != nil {
	return err
}
panel := admin.New(admin.Options{
	Title: "Shelf admin",
	Authorize: func(c *kit.Ctx) error {
		u, err := acc.Require(c) // signed out: to the sign-in page
		if err != nil {
			return err
		}
		if !isAdmin(u) {
			return kit.Error(http.StatusForbidden, "")
		}
		return nil
	},
},
	admin.Resource("links", links),
	admin.Resource("users", users, admin.ReadOnly()),
)
```

`NewTable`'s last argument is the list's order when nobody picked one, such as `"added_at DESC"`; `""` is the newest key first. It works with SQLite and PostgreSQL.

**`Authorize` is required.** It runs before every screen and every save or delete. Decide there who's an admin: a role in your users table, an `rbac` check, or a list of user IDs in the environment. An email address in the environment is only safe if the app verifies addresses: otherwise anyone can sign up with the address before its owner does.

A resource's options:

- **`admin.Label("People", "Person")`** names it in the menu and on its screens. The default comes from its name.
- **`admin.ReadOnly()`** shows records without changing them. **`admin.NoCreate()`** and **`admin.NoDelete()`** leave out one of those.

`Options.Path` is where the route is, `/admin` by default, and `Options.PerPage` how many records a list shows, 25 by default.

## The route

Add the panel to the app's plugins:

```go
ternApp(kit.WithPlugins(panel))
```

and give it a route that matches every path under it, such as `src/routes/admin/path.rest`:

```go
// src/routes/admin/path.rest/page.server.go
package admin

import (
	"github.com/Unfield/Tern/admin"
	"github.com/Unfield/Tern/kit"
)

func Load(c *kit.Ctx) (admin.Page, error) { return admin.Load(c) }

var Actions = admin.Actions
```

```svelte
<!-- src/routes/admin/path.rest/+page.svelte -->
<script lang="ts">
	import { Admin } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
</script>

<Admin {data} {form} />
```

The screens are pages of the app, inside its layouts, so they're rendered on the server and work without JavaScript. With it, the forms check their rules as the admin types, and deleting asks first. `/admin` is an overview with how many records each resource has; `/admin/links` lists them; `/admin/links/new` and `/admin/links/3` are the forms. A list's search, filters, sort and page are in its address, so they can be bookmarked and shared.

The `<Admin>` component takes the room it's given; a layout that keeps its pages narrow can widen them for it, as Shelf's does:

```css
main:has(:global(.tern-admin)) {
	max-width: 72rem;
}
```

## Data elsewhere

`NewTable` is one `admin.Store`. For records kept elsewhere, such as in another service, implement the interface:

```go
type Store[T any] interface {
	List(ctx context.Context, q admin.Query) (records []T, total int, err error)
	Get(ctx context.Context, key string) (T, error) // admin.ErrNotFound if there's none
	Create(ctx context.Context, record T) (key string, err error)
	Update(ctx context.Context, key string, record T, columns []string) error
	Delete(ctx context.Context, key string) error
}
```

A `Query` has the search text, the filters by column, the column to sort by, and the page's offset and limit. `Update` gets the columns the form edited, so it can leave the others alone.

## Errors

- A form that fails its rules or `Validate` shows each error next to its field, with what was typed.
- Saving a value that a unique column already has fails the form with a message, rather than an error page. So does deleting a record that others refer to with a foreign key. Both are recognized for SQLite and PostgreSQL.
- A record that doesn't exist, and a resource that isn't in the panel, are 404s.

The finished [Shelf](https://github.com/Unfield/Tern/tree/main/examples/shelf) has admin screens for its links and accounts, in `admin.go`.
