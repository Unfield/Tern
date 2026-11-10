# Server-rendering compatibility

Renders popular Svelte libraries in Tern's server renderer (QuickJS) and checks the HTML is identical to rendering the same bundle in Node.

```sh
pnpm --filter ssr-compat build              # builds the bundle and renders it in Node
go test -run TestCompat -v ./internal/ssr/  # renders it in QuickJS and compares
```

Add a library by writing a case in `src/cases/` and listing it in `src/entry.js` with text its HTML must contain. CI runs this suite on every push.
