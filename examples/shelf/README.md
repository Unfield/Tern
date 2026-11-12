# Shelf

A reading list: save links to read later, mark them read, and filter them, with accounts so each user has a shelf of their own. It's the app the [tutorial](../../docs/tutorial/README.md) builds, step by step.

```sh
pnpm install
pnpm dev
```

- `pnpm test` builds the app and runs the Go tests in `main_test.go`.
- `pnpm test:e2e` runs the browser tests in `tests/` against the production build.
- The database is `shelf.db`, or the file `$DATABASE` names.
