# 0024. Admin screens from Go structs

- Status: Accepted
- Date: 2026-11-12

## Context

Almost every app needs screens to look at and fix its own data: find a user, correct a typo in a record, delete something spam. Apps write them by hand, page by page, or do without and use a database client in production. Both are worse than they need to be: hand-written screens are a lot of code that's never the point of the app, and a database client skips the app's rules.

Tern already has what such screens are made of: structs with `validate` tags that forms check in Go and in the browser, Tern UI's form controls, and file-based routes with loads and actions.

## Decision

- **An `admin` package generates the screens from structs.** Each kind of record is a struct whose fields are its columns. Tags say how a field is shown: `admin:"list,search,filter,readonly,textarea,date,ref=users,unixms"`, plus the `label`, `description` and `validate` tags forms already use. A list has search, filters, sorting and pages; a form creates, edits and deletes, checked by the same rules as `kit.FormAction`, then the struct's `Validate` method.
- **Records live in a `Store`.** `admin.NewTable` is one for a SQL table, in SQLite or PostgreSQL. An app implements `Store` for data kept elsewhere, such as an API.
- **The panel is a plugin, and the app gives it a route.** A two-line `page.server.go` returns `admin.Load(c)` and `admin.Actions`, and the page renders Tern UI's `<Admin>`. The app chooses the path, its layout and who gets in, and the screens are ordinary pages: server-rendered, working without JavaScript, typed from Go.
- **`Authorize` is required.** A panel that can change everything must say who may use it; there is no default.
- **Explicit structs, not the database's schema.** The panel doesn't read tables to find columns: the struct is where an app says what admins may see and change, and a column it leaves out stays out.

## Consequences

- An app gets working admin screens from a struct per table and a route, and they follow its validation rules.
- The screens are generic: no custom widgets, dashboards or bulk actions. An app writes its own pages for those, next to the panel.
- References between resources are links (`ref=users`), not joins, so a list shows an owner's ID, not their name.
- Unique and foreign key violations are recognized from the messages of SQLite and PostgreSQL drivers, since `database/sql` has no common error type. Other drivers' violations are errors.
