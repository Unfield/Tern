# SQL queries

Write queries in SQL, and Tern writes the Go that runs them: typed functions, checked against the schema your migrations build, with every value sent as a bound parameter ([ADR 0028](adr/0028-typed-sql.md)). There's no ORM and nothing at run time but `database/sql`.

## Writing queries

Put `.sql` files in `queries/`, at the root of the app. Each query starts with a line that names it and says what it returns:

```sql
-- A user's links, the newest first.
-- name: LinksOf :many
SELECT id, url, title, read, added_at FROM links
WHERE user_id = :user_id
ORDER BY id DESC;

-- name: AddLink :one
INSERT INTO links (user_id, url, title) VALUES (:user_id, :url, :title)
RETURNING id, added_at;

-- name: MarkRead :execrows
UPDATE links SET read = :read WHERE id = :id AND user_id = :user_id;
```

`tern gen` writes `queries/queries.gen.go`, and so do `tern dev`, `tern build` and `tern check`; under `tern dev`, saving a query or a migration regenerates it. Commit it with the queries.

```go
q, err := queries.New(db) // SQLite or PostgreSQL

links, err := q.LinksOf(ctx, user.ID)               // []queries.LinksOfRow
added, err := q.AddLink(ctx, user.ID, url, title)   // queries.AddLinkRow
n, err := q.MarkRead(ctx, true, id, user.ID)        // rows changed
```

| Kind | Returns |
|---|---|
| `:one` | the row, or `queries.ErrNotFound` |
| `:many` | a slice of rows, empty rather than nil |
| `:exec` | an error |
| `:execrows` | how many rows changed |

A query of one column returns its values: `SELECT COUNT(*) AS n …` as `:one` is an `int64`. `q.WithTx(tx)` runs the queries in a transaction.

## Parameters

Parameters are named, `:name`, and always sent apart from the SQL, so nothing in them can change the query. They become the function's arguments, in the order they first appear, or a struct, `AddLinkParams`, when there are more than three. A name used twice is one argument.

Each parameter takes the type of the column it's compared with, inserted into or set to; `LIMIT :n` is an `int64`, and `:show = 'all'` a string. Inserting into or setting a column that allows NULL takes a pointer. Where the SQL doesn't tell, say it:

```sql
-- name: Since :many
-- param since time.Time
SELECT id FROM links WHERE julianday(added_at) > julianday(:since);
```

## Rows

A query's columns become a struct, `<Name>Row`, with the columns' names in Go's case (`added_at` is `AddedAt`, `user_id` is `UserID`) and in camelCase for JSON (`addedAt`, `userId`), so a load can return rows to a page as they are. Columns that allow NULL are pointers. Queries that return the same columns can share a type: `-- row Link` names it.

Columns come from the database, so an expression needs a name, `AS total`, and sometimes a type, when the query returns no row to tell it from:

```sql
-- name: NextID :one
-- column next *int64
SELECT max(id) + 1 AS next FROM links;
```

| Column type | Go type |
|---|---|
| `INTEGER`, `BIGINT`, `SERIAL` | `int64` |
| `TEXT`, `VARCHAR`, `UUID` | `string` |
| `REAL`, `DOUBLE`, `NUMERIC` | `float64` |
| `BOOLEAN` | `bool` |
| `TIMESTAMP`, `DATETIME`, `DATE` | `time.Time` |
| `BLOB`, `BYTEA` | `[]byte` |
| `JSON`, `JSONB` | `json.RawMessage` |

## How queries are checked

Code generation applies the migrations in `migrations/` to a scratch SQLite database in memory, and runs each query there in a transaction it rolls back. A column that doesn't exist, a table renamed in a migration, or SQL that doesn't parse stops the build with the query's file and line:

```
queries/links.sql:7: LinksOf: no such column: titel
```

Queries are written once with `:name` and run on SQLite or PostgreSQL: the package sends `?` to one and `$1` to the other. They're checked against SQLite, so on PostgreSQL write SQL both accept. For a query only PostgreSQL runs (`ILIKE`, `jsonb` operators), mark it `-- unchecked` and give each parameter and column its type; it runs as written:

```sql
-- name: Find :many
-- unchecked
-- param q string
-- column id int64
-- column title string
SELECT id, title FROM links WHERE title ILIKE '%' || :q || '%';
```

Queries and hand-written SQL can live side by side: nothing needs to move at once.
