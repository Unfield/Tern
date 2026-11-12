# 0025. Full-text search

- Status: Accepted
- Date: 2026-11-12

## Context

Apps need search: a user's own records, a team's documents, a site's content. The usual answer is a search service (Elasticsearch, Meilisearch, Typesense), which is one more thing to run and keep in step. Tern apps already have a database, and both databases Tern supports have full-text search built in: SQLite's FTS5 and PostgreSQL's `tsvector`. Content collections often have no database at all.

## Decision

- **A `search` package with one API over three stores:** an FTS5 table in SQLite, a table with a `tsvector` column and a GIN index in PostgreSQL, and an inverted index in memory. Each takes documents with an ID, a scope and text fields, and returns ranked hits.
- **Searches are for typing.** Each word of a query matches as a prefix, every word must match, and case and accents are ignored. A query's text is never query syntax: it's split into words, which become quoted prefixes in FTS5 and lexemes in a tsquery.
- **Scopes are part of the index**, so a user's search can't return another user's documents, whatever the query.
- **Snippets are segments, not HTML.** Each hit has every field's text with the matches marked, as `{text, match}` pieces. The page renders them as text with `<mark>`, so a document can't inject markup.
- **The app keeps the index in step**, with `Put` and `Delete` where its data changes, rather than with database triggers, which the package can't write for an app's own tables in both databases. The admin package gets `AfterSave` and `AfterDelete` for the changes it makes.
- **Tern UI's `SearchBox`** is a GET form with results as the user types: it works without JavaScript, through the results page.

## Consequences

- Search needs no service: an app that has a database has search, and one without can index in memory.
- Ranking and stemming differ a little between the stores: BM25 in SQLite and memory, `ts_rank` with stems in PostgreSQL.
- Typo tolerance, facets and synonyms are out of reach; an app that needs them uses a search service.
- An index can drift from the data if code changes rows without calling `Put`; `Reset` and indexing everything again repairs it, as Shelf does when it starts.
