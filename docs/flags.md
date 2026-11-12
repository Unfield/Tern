# Feature flags

The `flags` package turns a feature on for some users and not others, decided on the server: for the team that built it first, then for a share of users that grows, with a switch to turn it off for everyone at once. Flags live in the app's database, so they change without a deploy, and are read from memory, so checking one costs nothing.

## Flags

Define the flags in code, with the database and who each request is for:

```go
f, err := flags.New(db, flags.Options{Subject: flags.AccountsSubject},
	flags.Flag{Name: "new-editor", Description: "The editor with live previews"},
	flags.Flag{Name: "search", Description: "Search on the shelf", On: true, Percent: 100},
)
```

and add them to the app after the accounts plugin, since they're decided per user:

```go
ternApp(kit.WithPlugins(acc, f))
```

When the app starts, flags that aren't in the database yet are added as defined, which is off unless `On` is set; flags already there keep how they were set. The table is `tern_flags`, in SQLite or PostgreSQL.

## Checking a flag

```go
if f.Enabled(c, "new-editor") {
	// the new way
}
```

A flag that doesn't exist is off, and logged once, so removing a flag before the code that checks it is safe. For the page, return the decisions from a load:

```go
func LayoutLoad(c *kit.Ctx) (Layout, error) {
	return Layout{Flags: f.For(c, "new-editor", "search")}, nil
}
```

```svelte
{#if data.flags?.['new-editor']}<NewEditor />{:else}<Editor />{/if}
```

Decide on the server, and send the page only the answers: what the rules are, and who else gets what, stays there.

## Who gets it

A flag has:

- **On**, the switch. Off, nobody gets the feature, whatever else is set: turn it off when something goes wrong.
- **Users** and **Teams**: IDs, separated by commas, who always get it, such as the team that built it and the customers who asked for it.
- **Percent** of users, teams or visitors (**By**) who get it, from 0 to 100.

A rollout is sticky. Each user's place for a flag comes from a hash of the flag's name and the user's ID, so a user who gets the feature at 10% still has it at 30%, and keeps it on every request. Places differ between flags, so the same 10% don't get every new feature. Counted by team, everyone in a team gets the same answer. Counted by visitor, a cookie (`tern_visitor`) identifies the browser, for pages without accounts.

## Who a request is for

`Options.Subject` says who a request is for: `flags.AccountsSubject` is the signed-in user and their current team, from the [accounts](accounts.md) package. For accounts of your own, return the IDs yourself:

```go
Subject: func(c *kit.Ctx) flags.Subject {
	u := currentUser(c)
	if u == nil {
		return flags.Subject{}
	}
	return flags.Subject{User: strconv.FormatInt(u.ID, 10), Team: strconv.FormatInt(u.OrgID, 10)}
},
```

A signed-out visitor has no user or team, so only flags at 100%, or counted by visitor, are on for them.

## Changing flags

- **On the admin screens:** `f.Admin()` is a resource for the [admin panel](admin.md), with a list of the flags and a form for each:

  ```go
  admin.New(admin.Options{Authorize: onlyAdmins}, links, users, f.Admin())
  ```

- **In code:** `f.Set(ctx, flag)` saves a flag and `f.Remove(ctx, name)` deletes one; `f.List()` and `f.Get(name)` read them.

A change applies at once in the process that made it, and within `Options.Refresh` (10 seconds by default) in the others, which read the table again.

## In tern dev

The [dev tools](dev-tools.md) show which flags each page checked, and what each decided, in the Page tab.

## Testing

`Flag.Decide(subject, visitor)` is the decision without a request, for unit tests of who gets what. In tests of the whole app, `f.Set` turns a flag on or off before the requests that depend on it.
