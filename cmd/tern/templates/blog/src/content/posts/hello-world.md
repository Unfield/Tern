---
title: Hello, world
description: The first post on this blog.
date: 2026-11-01
---
This is a post in `src/content/posts`. Write Markdown with front matter, and Tern renders it to HTML in Go.

## What you get

- Posts sorted newest first, with excerpts and reading times
- Pages prerendered to static HTML by `tern build`
- An RSS feed at `/rss.xml`

```go
var Posts = &content.Collection[Post]{Dir: "posts"}
```
