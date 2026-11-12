# 0028. Typed SQL from .sql files

- Status: Accepted
- Date: 2026-11-12

## Context

Tern apps keep their data in SQLite or PostgreSQL through `database/sql`. Migrations are SQL files that `migrate` applies; queries are strings in Go, with `?` placeholders and a `Scan` into variables in the right order. That works, and it fails in quiet ways:

- **Injection** comes back whenever a query is built from strings, as filters and sort orders tempt people to.
- **Drift:** renaming a column in a migration breaks queries that nothing checks until they run.
- **Boilerplate:** each query repeats its columns three times (the `SELECT`, the struct, the `Scan`), in an order that must agree.

People reach for an ORM to fix this. ORMs hide the SQL that runs, make N+1 queries easy, and fight Go's types. The alternative that has worked in Go is sqlc's: the queries stay SQL, and code generation writes the Go.

## Decision

An app's queries live in `.sql` files in `queries/`. Each is named, and says what it returns:

```sql
-- name: LinksOf :many
SELECT id, url, title, read, added_at FROM links
WHERE user_id = :user ORDER BY id DESC;

-- name: AddLink :one
INSERT INTO links (user_id, url, title) VALUES (:user, :url, :title)
RETURNING id, added_at;

-- name: MarkRead :execrows
UPDATE links SET read = :read WHERE id = :id AND user_id = :user;
```

`tern gen` (and so `tern dev`, `build` and `check`) writes `queries/queries.gen.go`, a package with a function per query:

```go
func (q *Queries) LinksOf(ctx context.Context, user int64) ([]LinksOfRow, error)
func (q *Queries) AddLink(ctx context.Context, in AddLinkParams) (AddLinkRow, error)
func (q *Queries) MarkRead(ctx context.Context, read bool, id, user int64) (int64, error)
```

- **Kinds:** `:one` returns a row, and `queries.ErrNotFound` when there's none; `:many` a slice; `:exec` only an error; `:execrows` the number of rows changed.
- **Parameters** are named, `:name`, and always sent as bound parameters: a query file can't be put together from strings, so the generated code can't be injected into. With more than three, they come as a struct.
- **Types come from the schema.** Code generation applies the app's migrations to a scratch SQLite database in memory, runs each query there inside a transaction it rolls back, and reads the result's column types; a parameter compared with, inserted into or set to a column takes that column's type. Where neither tells, the query says: `-- param since time.Time`, or `-- column n int64` for an expression. Nullable columns are pointers.
- **Rows** are structs whose fields are the columns in Go's case, with JSON names in camelCase, so a load can return them to a page as they are; a query of one column returns its values directly.
- **Dialects:** queries are written once with `:name` parameters. The package rewrites them for the database it's given at run time (`?` for SQLite, `$1` for PostgreSQL), as `sqldb` does for Tern's own packages. Types are checked against SQLite, so the SQL must be what both accept when an app runs on PostgreSQL; queries that only PostgreSQL accepts can be typed by hand with `-- param` and `-- column` and are run as written.
- **Errors** name the `.sql` file and line: a query that doesn't parse, a column that doesn't exist, a parameter without a type.

## Consequences

- The schema is checked at build time: renaming a column in a migration fails `tern check` at every query that used it.
- No ORM, no reflection at run time: the generated code is `database/sql`, readable, and what a person would write.
- Apps can mix generated queries and hand-written ones; nothing requires moving.
- Type inference covers what queries usually do. The escape hatches (`-- param`, `-- column`) are for the rest; the error says which to add.
- PostgreSQL-only SQL (`ILIKE`, `jsonb` operators, `RETURNING` lists with casts) needs annotations, since the scratch database is SQLite. A later version can type against a PostgreSQL database named in the environment.
