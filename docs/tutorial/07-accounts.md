# 7. Accounts

So far everyone shares one shelf. Now each user gets their own: they sign up, sign in, and see only their links.

Tern's `accounts` package is user accounts, done: it keeps users and sessions in your database, hashes passwords, limits how fast anyone can guess them, and has screens in Tern UI for each step. This step uses sign-up and sign-in with a password; the same package also does passkeys, email verification, password resets and two-step sign-in, in [Accounts](../accounts.md).

## Links that belong to someone

Each link needs an owner. Since the first migration has already run, add a second one:

```sh
go tool tern migrate new links per user
```

```sql
-- Links saved before accounts belong to no one.
ALTER TABLE links ADD COLUMN user_id INTEGER NOT NULL DEFAULT 0;
CREATE INDEX links_by_user ON links (user_id, id);

-- +down
DROP INDEX links_by_user;
ALTER TABLE links DROP COLUMN user_id;
```

## Accounts in the app

`internal/shelf/shelf.go` sets up accounts when it opens the database, and every function takes the user's ID, so a query can only ever see that user's links. Here's the whole file:

```go
// Package shelf keeps the links in a SQLite database.
package shelf

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/Unfield/Tern/accounts"
	_ "modernc.org/sqlite" // the SQLite driver, in pure Go
)

// Link is a page saved to read later.
type Link struct {
	ID      int64     `json:"id"`
	URL     string    `json:"url"`
	Title   string    `json:"title"`
	Read    bool      `json:"read"`
	AddedAt time.Time `json:"addedAt"`
}

// ErrNotFound is returned for a link that doesn't exist, or belongs to
// someone else.
var ErrNotFound = errors.New("link not found")

var (
	// DB is the database, opened by Open.
	DB *sql.DB
	// Accounts are the app's users, kept in the same database.
	Accounts *accounts.Accounts
)

// Open opens the SQLite database at path, creating it if needed, and sets
// up accounts in it.
func Open(ctx context.Context, path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return err
	}
	acc, err := accounts.New(db, accounts.Options{})
	if err != nil {
		return err
	}
	if err := acc.Migrate(ctx); err != nil {
		return err
	}
	DB, Accounts = db, acc
	return nil
}

// List returns a user's links, the newest first. show is "unread", "read"
// or anything else for all of them.
func List(ctx context.Context, userID int64, show string) ([]Link, error) {
	query := `SELECT id, url, title, read, added_at FROM links WHERE user_id = ?`
	switch show {
	case "unread":
		query += ` AND NOT read`
	case "read":
		query += ` AND read`
	}
	rows, err := DB.QueryContext(ctx, query+` ORDER BY id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []Link
	for rows.Next() {
		var l Link
		if err := rows.Scan(&l.ID, &l.URL, &l.Title, &l.Read, &l.AddedAt); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

// Add saves a link for a user. Without a title, the address is the title.
func Add(ctx context.Context, userID int64, url, title string) (Link, error) {
	if title == "" {
		title = url
	}
	l := Link{URL: url, Title: title, AddedAt: time.Now().UTC()}
	err := DB.QueryRowContext(ctx, `INSERT INTO links (user_id, url, title, added_at) VALUES (?, ?, ?, ?) RETURNING id`,
		userID, l.URL, l.Title, l.AddedAt).Scan(&l.ID)
	return l, err
}

// Get returns one of a user's links.
func Get(ctx context.Context, userID, id int64) (Link, error) {
	var l Link
	err := DB.QueryRowContext(ctx, `SELECT id, url, title, read, added_at FROM links WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&l.ID, &l.URL, &l.Title, &l.Read, &l.AddedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	return l, err
}

// SetRead marks one of a user's links read or unread.
func SetRead(ctx context.Context, userID, id int64, read bool) error {
	return changed(DB.ExecContext(ctx, `UPDATE links SET read = ? WHERE id = ? AND user_id = ?`, read, id, userID))
}

// Delete removes one of a user's links.
func Delete(ctx context.Context, userID, id int64) error {
	return changed(DB.ExecContext(ctx, `DELETE FROM links WHERE id = ? AND user_id = ?`, id, userID))
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

`acc.Migrate` creates the accounts' own tables. Accounts is a plugin too: it reads the session on every request, before your code runs. In `main.go`, open the database with a context, and add accounts after the migrations. The database's path can now come from `$DATABASE`, so a server can keep it where it likes:

```go
package main

import (
	"context"
	"log"
	"os"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/migrate"

	"shelf/internal/shelf"
	"shelf/migrations"
)

func main() {
	path := os.Getenv("DATABASE")
	if path == "" {
		path = "shelf.db"
	}
	if err := shelf.Open(context.Background(), path); err != nil {
		log.Fatal(err)
	}
	app, err := ternApp(kit.WithPlugins(
		migrate.Plugin(shelf.DB, migrations.FS),
		shelf.Accounts,
	))
	if err != nil {
		log.Fatal(err)
	}
	if err := app.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
```

## Signing up and signing in

Create `src/routes/signup/page.server.go`:

```go
package signup

import (
	"net/http"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"shelf/internal/shelf"
)

type NewAccount struct {
	Name     string `form:"name" validate:"required,max=80"`
	Email    string `form:"email" validate:"required,email,max=254"`
	Password string `form:"password" validate:"required,min=10,max=200"`
}

var Actions = kit.Actions{
	"default": kit.FormAction(func(ctx *kit.Ctx, in NewAccount) (struct{}, error) {
		// SignUp creates the account and signs it in.
		if _, err := shelf.Accounts.SignUp(ctx, in.Email, in.Name, in.Password); err != nil {
			// A taken email, say, becomes a message on the form.
			return struct{}{}, accounts.FormError(ctx, err)
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/")
	}),
}
```

And its page, `src/routes/signup/+page.svelte`, with Tern UI's sign-up screen:

```svelte
<script lang="ts">
	import { SignUp } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { form }: PageProps = $props();
</script>

<svelte:head><title>Create an account · Shelf</title></svelte:head>

<SignUp rules={forms.default} {form} signInHref="/login" />
```

A form without `?/name` in its action posts to the `"default"` action. Signing in is the same shape, with a `next` address to return to. Create `src/routes/login/page.server.go`:

```go
package login

import (
	"net/http"
	"strings"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"

	"shelf/internal/shelf"
)

type Credentials struct {
	Email    string `form:"email" validate:"required,email"`
	Password string `form:"password" validate:"required"`
	Remember bool   `form:"remember"`
	Next     string `form:"next"`
}

type Page struct {
	Next string `json:"next"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	next := safeNext(ctx.URL().Query().Get("next"))
	if accounts.Current(ctx) != nil {
		return Page{}, kit.Redirect(http.StatusSeeOther, next)
	}
	return Page{Next: next}, nil
}

var Actions = kit.Actions{
	"default": kit.FormAction(func(ctx *kit.Ctx, in Credentials) (struct{}, error) {
		if _, err := shelf.Accounts.SignIn(ctx, in.Email, in.Password, in.Remember); err != nil {
			return struct{}{}, accounts.FormError(ctx, err)
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, safeNext(in.Next))
	}),
}

// safeNext keeps the redirect after signing in on this site: "next" comes
// from the URL, and anyone can make a link with one.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}
```

`src/routes/login/+page.svelte`:

```svelte
<script lang="ts">
	import { SignIn } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { data, form }: PageProps = $props();
</script>

<svelte:head><title>Sign in · Shelf</title></svelte:head>

<SignIn rules={forms.default} {form} next={data.next} remember signUpHref="/signup" />
```

A wrong password and an unknown email get the same message, so the form doesn't tell anyone which addresses have accounts.

Signing out changes state, so it's a form post rather than a link. Create `src/routes/logout/page.server.go`:

```go
package logout

import (
	"net/http"

	"github.com/Unfield/Tern/kit"

	"shelf/internal/shelf"
)

// Signing out changes state, so it's a form post, not a link.
var Actions = kit.Actions{
	"default": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		if err := shelf.Accounts.SignOut(ctx); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, kit.Redirect(http.StatusSeeOther, "/login")
	}),
}
```

Actions belong to a page, so the route needs a `+page.svelte` too. It also helps anyone who opens `/logout` directly. `src/routes/logout/+page.svelte`:

```svelte
<script lang="ts">
	import { Button } from '@ternkit/tern/ui';
</script>

<svelte:head><title>Sign out · Shelf</title></svelte:head>

<form method="POST">
	<Button type="submit">Sign out</Button>
</form>
```

## Who's signed in

A layout's load runs for every page below it, so the layout is where to say who's signed in. Create `src/routes/layout.server.go`:

```go
package routes

import (
	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"
)

type Layout struct {
	User *accounts.User `json:"user"`
}

func LayoutLoad(ctx *kit.Ctx) (Layout, error) {
	return Layout{User: accounts.Current(ctx)}, nil
}
```

`accounts.Current` is the signed-in user, or nil. Replace `src/routes/+layout.svelte` with a header that shows the user and a sign-out button:

```svelte
<script lang="ts">
	import '@ternkit/tern/ui/theme.css';
	import { navigating } from '@ternkit/tern';
	import { Button, Spinner, Toaster } from '@ternkit/tern/ui';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();
</script>

<div class="tern shell">
	<header>
		<a class="brand" href="/">Shelf</a>
		{#if navigating.current}<Spinner size="1rem" label="Loading" />{/if}
		<nav aria-label="Account">
			{#if data.user}
				<span class="who">{data.user.name}</span>
				<form method="POST" action="/logout">
					<Button type="submit" variant="ghost" size="sm">Sign out</Button>
				</form>
			{:else}
				<a href="/login">Sign in</a>
			{/if}
		</nav>
	</header>

	<main>
		{@render children()}
	</main>
</div>

<Toaster />

<style>
	:global(body) {
		margin: 0;
		background: var(--tern-bg);
	}
	.shell {
		min-height: 100vh;
	}
	header {
		display: flex;
		align-items: center;
		gap: 1rem;
		padding: 0.75rem 1.5rem;
		border-bottom: 1px solid var(--tern-border);
		background: var(--tern-surface);
	}
	.brand {
		color: var(--tern-fg);
		font-weight: 650;
		text-decoration: none;
	}
	nav {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		margin-left: auto;
		font-size: 0.875rem;
	}
	nav a {
		color: var(--tern-fg);
	}
	.who {
		color: var(--tern-muted);
	}
	main {
		max-width: 40rem;
		margin: 2.5rem auto;
		padding: 0 1.5rem;
	}
	main :global(.tern-auth) {
		margin: 0 auto;
	}
</style>
```

## Only your links

Last, the shelf's loads and actions need a user. `shelf.Accounts.Require(ctx)` returns the signed-in user; signed out, it returns a redirect to `/login?next=…`, which comes back to the same page after signing in. Return it as the error, and the visitor is sent to sign in.

In `src/routes/page.server.go`:

```go
func Load(ctx *kit.Ctx) (Data, error) {
	// Signed out, Require returns a redirect to the sign-in page.
	user, err := shelf.Accounts.Require(ctx)
	if err != nil {
		return Data{}, err
	}
	show := ctx.URL().Query().Get("show")
	if show != "read" && show != "all" {
		show = "unread"
	}
	links, err := shelf.List(ctx, user.ID, show)
	if err != nil {
		return Data{}, err
	}
	return Data{Links: links, Show: show}, nil
}
```

```go
var Actions = kit.Actions{
	"add": kit.FormAction(func(ctx *kit.Ctx, in NewLink) (Added, error) {
		user, err := shelf.Accounts.Require(ctx)
		if err != nil {
			return Added{}, err
		}
		link, err := shelf.Add(ctx, user.ID, in.URL, strings.TrimSpace(in.Title))
		if err != nil {
			return Added{}, err
		}
		return Added{Link: link}, nil
	}),
}
```

And the same in `src/routes/links/id.param.integer/page.server.go`, where every function now passes `user.ID`:

```go
func Load(ctx *kit.Ctx) (Data, error) {
	user, err := shelf.Accounts.Require(ctx)
	if err != nil {
		return Data{}, err
	}
	link, err := shelf.Get(ctx, user.ID, id(ctx))
	if errors.Is(err, shelf.ErrNotFound) {
		return Data{}, kit.NotFound()
	}
	if err != nil {
		return Data{}, err
	}
	return Data{Link: link}, nil
}
```

```go
var Actions = kit.Actions{
	"read": kit.FormAction(func(ctx *kit.Ctx, in Toggle) (struct{}, error) {
		user, err := shelf.Accounts.Require(ctx)
		if err != nil {
			return struct{}{}, err
		}
		err = shelf.SetRead(ctx, user.ID, id(ctx), in.Read)
		if errors.Is(err, shelf.ErrNotFound) {
			return struct{}{}, kit.NotFound()
		}
		return struct{}{}, err
	}),
	"delete": kit.Action(func(ctx *kit.Ctx) (struct{}, error) {
		user, err := shelf.Accounts.Require(ctx)
		if err != nil {
			return struct{}{}, err
		}
		err = shelf.Delete(ctx, user.ID, id(ctx))
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

Check the user in actions as well as in loads. An action is a POST that anyone can send, whether or not they ever loaded the page.

Open the shelf: you're sent to sign in. Create an account, and you're back on an empty shelf of your own; the links from before belong to no one. Someone else's link is a 404 for you, as if it didn't exist, because `Get` only finds your own.

Next: [Tests](08-tests.md).
