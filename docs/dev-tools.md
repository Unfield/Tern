# Dev tools

Under `tern dev`, every page has a **T** button in its bottom-left corner. It opens a panel about the page you're on, and errors open over the page as they happen. None of it is in a build.

## The panel

- **Page:** the route, the address, the status and the params, and the route's files: each layout and the page, with its `page.server.go` or `layout.server.go` at the line of its load. Click a file to open it in your editor. Below them are the [feature flags](flags.md) the page checked and what each decided, and the data each load returned, as a tree.
- **Requests:** the page's requests, its loads, client-side navigations and form actions, newest first. For each, the waterfall of its spans, such as each load and the render, and what it logged.
- **Errors:** every error on the page, to open again.

The panel follows client-side navigations. **Hide** hides the button until an error happens; `ternDev.show()` in the browser's console brings it back.

## Errors

An error opens over the page:

- **From the server**, such as a load that failed: the message and the app's own frames of the stack, each with the source around the line and a button that opens it in your editor. Go's and other packages' frames are folded away. An error without a stack, such as one made with `errors.New`, shows the load that returned it, at the line with its message if there's one. **Show the request** opens its waterfall and logs.
- **From the browser**, such as an exception in an event handler, with its stack. A `.svelte` frame opens the file, without a line: in the browser, lines are those of the compiled component.
- **From the build**, when the Go code doesn't compile: each compiler error, with its source. Fix it and save, and the page reloads by itself.

Esc closes the overlay. Several errors are numbered, with arrows between them.

## Your editor

Files open in the editor in `TERN_EDITOR`, or else the first of Zed, Cursor and VS Code that is installed:

```sh
TERN_EDITOR=zed go tool tern dev
TERN_EDITOR="code -g" go tool tern dev
TERN_EDITOR="idea --line {line} {file}" go tool tern dev
```

Without `{file}`, the command gets `file:line` at its end. `tern dev` opens files only in the project, and answers only the app's own pages.

## Where the data comes from

The overlay asks `tern dev`, which sees everything the app does in development: the requests it passes to the app, and the app's [logs](logging.md), [spans](tracing.md) and [error reports](errors.md). Each response tells the page its request ID. The full log, with every request, is at `/__tern/dev/logs`.
