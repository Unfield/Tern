# Search

The `search` package is full-text search for an app's data and content. It keeps an index in the app's database, SQLite with FTS5 or PostgreSQL with `tsvector`, or in memory, and finds documents as people type, with snippets that mark what matched. Tern UI's `SearchBox` shows them.

## An index

An index has fields, the most important first, and documents with an ID, a scope and the text of each field:

```go
idx, err := search.New(db, "links", search.Options{
	Fields: []search.Field{{Name: "title", Boost: 2}, {Name: "url"}},
})
if err != nil {
	return err
}
if err := idx.Migrate(ctx); err != nil { // creates its table: tern_search_links
	return err
}
```

- **`Boost`** weighs matches in a field: a word in a title with `Boost: 2` counts twice as much as in a body.
- **`Language`** is PostgreSQL's text search configuration, for stems such as "running" matching "run": `"english"` by default, `"simple"` for none.
- **`search.NewMemory(opts)`** keeps the index in memory instead, for content collections and other documents that fit, indexed when the app starts.

Keep the index up to date where the data changes:

```go
err := idx.Put(ctx, search.Doc{
	ID:     strconv.FormatInt(link.ID, 10),
	Scope:  "user:" + strconv.FormatInt(userID, 10),
	Fields: map[string]string{"title": link.Title, "url": link.URL},
})
err = idx.Delete(ctx, id)
```

`Put` replaces a document with the same ID. `Reset` empties the index, to index everything again, such as when the app starts.

## Searching

```go
res, err := idx.Search(ctx, search.Query{Text: q, Scope: "user:7", Limit: 20})
for _, hit := range res.Hits {
	hit.ID                  // the document
	hit.Snippets["title"]   // []search.Segment, with the matches marked
}
res.Total // how many match in all
```

- **As people type:** every word of the query must match, as a word or the start of one, so "go rel" finds "Go 1.26 is released". Case and accents don't matter: "creme" finds "Crème".
- **Safe input:** quotes, operators and other syntax in the query are only text.
- **Scopes:** a query with a `Scope` finds only that scope's documents, such as one user's or one team's; documents of other scopes are never in its results. Without one, it searches every document.
- **Ranking:** BM25 in SQLite and in memory, `ts_rank` in PostgreSQL, with each field's boost.
- **Snippets:** each hit has every field's text, with matches marked, cut to a few words around the first match when it's long. They're segments of text, not HTML, so nothing in a document can end up as markup on the page.

## A search box

`SearchBox` is a search form that shows results as people type:

```svelte
<script lang="ts">
	import { api } from '$api';
	import { SearchBox } from '@ternkit/tern/ui';
</script>

<SearchBox action="/search" hotkey="/" search={(q, signal) => api('GET /api/search', { query: { q }, signal })} />
```

- **`search`** returns the results for a query: items with an `href`, a `title` and an optional `snippet`, plain or as segments. It gets an `AbortSignal`, so a slower answer to an older query never replaces a newer one.
- **`action`** is the results page. Enter goes there with `?q=`, and so does the form without JavaScript, so the page also works without it. The list ends with a link to all the results.
- **Keys:** ↓ and ↑ move through the results, Enter opens one, Esc closes the list and then clears the box. **`hotkey`**, such as `"/"`, focuses the box from anywhere on the page.

The endpoint behind it is a [typed endpoint](guide.md#typed-endpoints) that returns the hits for the signed-in user:

```go
// src/routes/api/search/server.go
type Query struct {
	Q string `json:"q"`
}

type Result struct {
	Href    string           `json:"href"`
	Title   []search.Segment `json:"title"`
	Snippet []search.Segment `json:"snippet"`
}

func GET(ctx *kit.Ctx, in Query) ([]Result, error) {
	user := accounts.Current(ctx)
	if user == nil {
		return nil, kit.Error(http.StatusUnauthorized, "Sign in to search.")
	}
	res, err := idx.Search(ctx, search.Query{Text: in.Q, Scope: "user:" + strconv.FormatInt(user.ID, 10), Limit: 8})
	if err != nil {
		return nil, err
	}
	out := []Result{}
	for _, h := range res.Hits {
		out = append(out, Result{Href: "/links/" + h.ID, Title: h.Snippets["title"], Snippet: h.Snippets["url"]})
	}
	return out, nil
}
```

On the results page, `Highlight` shows a snippet with its matches marked:

```svelte
{#each data.results as r (r.href)}
	<a href={r.href}><Highlight text={r.title} /></a>
{/each}
```

## Content collections

For [Markdown content](guide.md#markdown-and-content), index the entries in memory when the app starts. `search.Text` is an entry's HTML as text, without its tags:

```go
idx, _ := search.NewMemory(search.Options{Fields: []search.Field{{Name: "title", Boost: 3}, {Name: "body"}}})
entries, err := posts.All(ctx)
for _, e := range entries {
	err = idx.Put(ctx, search.Doc{ID: e.Slug, Fields: map[string]string{"title": e.Data.Title, "body": search.Text(e.HTML)}})
}
```

## Admin screens

The [admin screens](admin.md) change records directly: tell the index with `admin.AfterSave` and `admin.AfterDelete`, as Shelf does:

```go
admin.Resource("links", links,
	admin.AfterSave(func(ctx context.Context, key string) error { return reindexLink(ctx, key) }),
	admin.AfterDelete(func(ctx context.Context, key string) error { return idx.Delete(ctx, key) }),
)
```

The finished [Shelf](https://github.com/Unfield/Tern/tree/main/examples/shelf) searches each user's links, with a box on the shelf and a results page at `/search`.
