# create-tern

Creates a [Tern](https://github.com/Unfield/Tern) app: Svelte on the client, Go on the server.

```sh
npm create tern@latest
```

It asks where to put the project, which template to start from, whether to style it with Tern UI, and whether to create a git repository and install packages. To skip the questions, pass a directory and flags:

```sh
npm create tern@latest my-app -- -template blog -ui -git -install
```

Tern needs [Go](https://go.dev/dl/). This package runs `tern new` from the matching Tern release, the same as:

```sh
go run github.com/Unfield/Tern/cmd/tern@latest new my-app
```
