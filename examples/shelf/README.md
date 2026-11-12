# Shelf

A reading list: save links to read later, mark them read, and filter them, with accounts so each user has a shelf of their own. It's the app the [tutorial](../../docs/tutorial/README.md) builds, step by step.

```sh
pnpm install
pnpm dev
```

- `pnpm test` builds the app and runs the Go tests in `main_test.go`.
- `pnpm test:e2e` runs the browser tests in `tests/` against the production build.
- The database is `shelf.db`, or the file `$DATABASE` names.
- Admin screens at `/admin`, for the account IDs in `SHELF_ADMINS` (such as `SHELF_ADMINS=1`): `admin.go` describes the links and accounts they manage, as in [Admin screens](../../docs/admin.md).
- Assistants such as Claude can use the shelf through MCP at `/mcp`: `assistants.go` has the tools and the sign-in, described in [Apps for assistants](../../docs/mcp-server.md). Set `BASE_URL` to the app's public address.
