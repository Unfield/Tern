# 9. Ship it

## Build

```sh
npm run build
```

`tern build` builds the client, compiles the server renderer to Go, and builds the server with all of it embedded: `build/shelf`, one file. It needs neither Node nor the project to run. Try it:

```sh
PORT=3000 ./build/shelf
```

Open <http://localhost:3000>. It starts on an empty `shelf.db`, applies both migrations, and serves the app, with pages rendered in Go and assets compressed and cached.

## Deploy

`tern deploy` builds the app for a server and puts it there over SSH. You need a Linux server with systemd, which most are, that you can reach as root or with sudo. Name it in `tern.json`, in the project:

```json
{
  "deploy": {
    "production": {
      "provider": "ssh",
      "host": "root@203.0.113.5",
      "domains": ["shelf.example.com"]
    }
  }
}
```

Point the domain's DNS at the server, then:

```sh
go tool tern deploy
```

The first time, it creates a system user and a systemd service for the app. Every time, it builds for the server's architecture, uploads the binary as a new release, starts it next to the old one, and switches over once the new one is ready. If the new release fails to start, the old one keeps serving. With `domains`, the app serves HTTPS itself, with certificates from Let's Encrypt, so the server needs nothing else.

The app runs in `/srv/shelf/shared`, which stays the same across releases, so `shelf.db` is kept there.

- **`go tool tern deploy -logs`** follows the app's logs.
- **`go tool tern deploy -rollback`** goes back to the release before.
- **Elsewhere:** [Deployment](../deployment.md) covers Fly.io, container registries, the `Dockerfile` the project came with, systemd by hand, and running behind a reverse proxy.

## What's next

You've built an app with pages and layouts, data from Go with generated types, a database with migrations, validated forms, accounts, and tests, and shipped it. Some ways to take Shelf further, and where to read about them:

- **Fetch the page's title** when a link is saved, in a [background job](../guide.md#background-jobs), so saving stays instant.
- **Tags**, with a many-to-many table and a filter in the URL beside `show`.
- **Sign in with a passkey**, verify emails, and let users reset their passwords: [Accounts](../accounts.md).
- **Live updates** across a user's tabs and devices: [Realtime](../guide.md#realtime).
- **Translations** of the interface: [Translations](../guide.md#translations).
- **Let assistants use it:** save links from Claude or ChatGPT, with tools that run as the user: [Apps for assistants](../mcp-server.md). The finished Shelf has them.

The [guide](../guide.md) covers all of Tern, and the finished Shelf is in [examples/shelf](../../examples/shelf).
