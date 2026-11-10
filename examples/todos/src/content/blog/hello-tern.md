---
title: Hello, Tern
date: 2026-11-01
tags: [announcement]
---
This blog is a **content collection**: Markdown files in `src/content/blog`, read by a Go `Load` function and prerendered.

## Why Markdown in Go?

Rendering happens on the server with [goldmark](https://github.com/yuin/goldmark), so pages ship plain HTML.

```go
var posts = &content.Collection[Post]{Dir: "blog"}
```
