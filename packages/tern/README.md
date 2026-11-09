# @ternkit/tern

The client runtime and Vite plugin for [Tern](https://github.com/Unfield/Tern), a full-stack framework with Svelte 5 on the client and Go on the server.

You normally get this package from a new project rather than installing it by hand:

```sh
go run github.com/Unfield/Tern/cmd/tern@latest new my-app
```

## What it provides

**`@ternkit/tern`**: the API you use in components.

```svelte
<script lang="ts">
	import { page, navigating, goto, enhance } from '@ternkit/tern';
</script>
```

| Export | Purpose |
|---|---|
| `page` | reactive state for the current page: `url`, `params`, `route`, `status`, `error`, `data`, `form` |
| `navigating` | the navigation in progress, or `null` |
| `goto`, `invalidateAll`, `preloadData` | client-side navigation and data |
| `enhance`, `applyAction` | progressive enhancement for forms that post to Go actions |

**`@ternkit/tern/vite`**: the Vite plugin. It includes the Svelte plugin, so it is the only plugin a Tern app needs.

```js
// vite.config.js
import { tern } from '@ternkit/tern/vite';
import { defineConfig } from 'vite';

export default defineConfig({ plugins: [tern()] });
```

## Requirements

- Svelte 5.46 or newer
- Vite 8
- The matching version of the Tern Go module and CLI

## Documentation

See the [Tern guide](https://github.com/Unfield/Tern/blob/main/docs/guide.md).

## License

MIT
