# Tutorial

This tutorial builds **Shelf**, a reading list: you save links to read later, mark them read, and filter them. Then you add accounts, so every user has a shelf of their own, write tests for it, and ship it as one binary to a server.

Each step adds one idea and ends with an app that runs. Along the way you use most of what a Tern app is made of:

1. [Create the app](01-create.md): a new project, and the dev server
2. [Your first page](02-first-page.md): a Go load, and the types it gives your Svelte page
3. [A database](03-database.md): SQLite, and migrations
4. [Saving links](04-forms.md): a form action, validated from Go struct tags
5. [A page for each link](05-link-pages.md): a route parameter, not-found pages, more actions
6. [Filtering](06-filters.md): state in the URL
7. [Accounts](07-accounts.md): sign-up, sign-in, and data that belongs to its user
8. [Tests](08-tests.md): the whole app, tested from Go
9. [Ship it](09-ship.md): one binary, deployed over SSH

The finished app is [examples/shelf](../../examples/shelf), if you want to compare.

## What you need

- **Go 1.26 or later**, and **Node.js 22 or later**. Node builds the client in development; the app you ship runs without it.
- Some Go, and some Svelte or another component framework. You don't need to know SvelteKit, but if you do, much of this will look familiar: [Coming from SvelteKit](../migrating-from-sveltekit.md) lists the differences.
