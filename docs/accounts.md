# User accounts

The `accounts` package signs users up and in, with a password or a passkey, and keeps their accounts: email verification, password resets, and the devices they're signed in on. Tern UI has a screen for each step. [examples/accounts](../examples/accounts) is a working app with all of them.

## Setup

```go
db, err := sql.Open("sqlite", "file:app.db?_pragma=journal_mode(WAL)")
acc, err := accounts.New(db, accounts.Options{
	Send:   accounts.SendWith(mailer, queue), // verification and reset emails
	RPName: "Acme",                           // the name browsers show for passkeys
})
if err := acc.Migrate(ctx); err != nil { ... }
app, err := ternApp(kit.WithPlugins(acc))
```

`Migrate` creates its tables if they don't exist: `tern_users`, `tern_sessions`, `tern_tokens` (email links, passkey challenges and sign-ins waiting for a code), `tern_passkeys`, `tern_two_step`, `tern_recovery_codes`, `tern_identities`, the teams' `tern_teams`, `tern_members` and `tern_invitations`, and the audit log's `tern_audit`. Tables from an earlier release are kept as they are. Keep `acc` where your loads and actions can reach it, such as a package variable.

| Option | Default | |
|---|---|---|
| `Send` | none | delivers emails. `SendWith(mailer, queue)` sends them with the `mail` package through a `jobs` queue, so they're retried. Without it, `SendVerification` and `RequestPasswordReset` return `ErrNoEmail`. |
| `BaseURL` | the request's | the app's address for links in emails and for passkeys, such as `https://example.com`. Set it in production. |
| `SignInPath` | `/login` | where `Require` sends signed-out users |
| `VerifyPath`, `ResetPath` | `/verify-email`, `/reset-password` | the pages the emails link to, with `?token=` |
| `Cookie` | `tern_session` | the session cookie's name |
| `SessionLength` | 30 days | how long a "keep me signed in" session lasts without use. Other sessions last a day. Sessions in use stay signed in. |
| `PasskeyPath` | `/_tern/passkeys` | where the passkey endpoints are |
| `TwoStepPath` | `/login/code` | the page that asks for the two-step code |
| `Issuer` | `RPName`, then the host | the app's name in authenticator apps |
| `NoRateLimits` | false | turns off the built-in limits on attempts |
| `TeamRoles` | none | team roles of the app's own, besides owner, admin and member |
| `NewTeamPath` | `/teams/new` | where `RequireTeam` sends users who aren't in a team |
| `InvitePath` | `/invitation` | the page invitation emails link to, with `?token=` |
| `OnDeleteTeam` | none | deletes the app's data of a team before the team is deleted |
| `AuditFor` | a year | how long the audit log keeps events; negative turns it off |

## Who is signed in

- `accounts.Current(ctx)` is the signed-in `*accounts.User`, or nil.
- `acc.Require(ctx)` is the user, or a redirect to the sign-in page with `?next=` set to come back. Return its error from a load or action.

To keep a route to signed-in users, give the app `acc.Guard()` and the route `var Access = kit.SignedIn` ([who may use a route](guide.md#who-may-use-a-route)); a group's rule covers every page in it:

```go
app, err := ternApp(kit.WithPlugins(acc), acc.Guard())
```

Loads still call `acc.Require` to get the user, as here in a layout:

```go
func LayoutLoad(ctx *kit.Ctx) (Account, error) {
	u, err := acc.Require(ctx)
	if err != nil {
		return Account{}, err
	}
	return Account{Email: u.Email}, nil
}
```

## The pages

Each page is a Tern UI block and a small action. Return `accounts.FormError(ctx, err)` from actions: it turns the errors users cause into form errors in their language and passes other errors through.

| Page | Block | Action calls |
|---|---|---|
| `/signup` | `SignUp` | `acc.SignUp(ctx, email, name, password)`, then `acc.SendVerification(ctx)` |
| `/login` | `SignIn passkey remember forgotHref="/forgot-password"` | `acc.SignIn(ctx, email, password, remember)`; on `ErrTwoStepRequired`, redirect to `/login/code` |
| `/login/code` | `TwoStepCode` | `acc.CompleteSignIn(ctx, code)`; the load sends users without `acc.AwaitingCode(ctx)` back to `/login` |
| `/forgot-password` | `ForgotPassword` | `acc.RequestPasswordReset(ctx, email)`; return `{ sent: true }` |
| `/reset-password` | `ResetPassword token={data.token}` | `acc.ResetPassword(ctx, token, password)` |
| `/verify-email` | none: the load calls `acc.VerifyEmail(ctx, token)` | |
| `/account` | `Devices`, `Passkeys`, `TwoStep`, `LinkedAccounts` | `acc.Revoke`, `acc.SignOutOthers`, `acc.RemovePasskey`, `acc.ChangePassword`, `acc.SetName`, `acc.DeleteAccount` |
| sign out | a form posting to an action | `acc.SignOut(ctx)` |

## Passwords

Passwords are hashed with argon2id with the parameters OWASP recommends (19 MiB, 2 passes). bcrypt hashes from another system also work: import them into `tern_users.password` and they're rehashed with argon2id when the user next signs in.

Signing in takes the same time whether or not the email has an account, and its error doesn't say which part was wrong. Add `plugins/ratelimit` to slow down guessing.

Changing or resetting a password signs out the user's other devices.

## Email verification and password resets

`acc.SendVerification(ctx)` emails the signed-in user a link to `VerifyPath`; it's valid for two days. `acc.RequestPasswordReset(ctx, email)` emails a link to `ResetPath`, valid for an hour, and does nothing for an email without an account, so the page can't be used to find out who has one. Each link works once.

The emails' texts are Tern's own (`tern.accounts.verify.*`, `tern.accounts.reset.*`), in the request's language: translate them in `src/locales` like the app's messages. To send your own emails instead, pass a `Send` function: it gets an `accounts.Email` with the recipient, the link, a subject, and text and HTML bodies.

## Two-step sign-in

Users can ask for a code from an authenticator app (any app for TOTP, RFC 6238) after their password:

1. `acc.SetUpTwoStep(ctx)` makes a secret and returns a QR code, the `otpauth://` link and the secret to type. `acc.PendingTwoStepSetup(ctx)` returns it again after a reload.
2. `acc.ConfirmTwoStep(ctx, code)` turns it on with a code from the app, and returns ten recovery codes to show once. `acc.RecoveryCodes(ctx)` replaces them, and `acc.RecoveryCodesLeft(ctx)` counts the unused ones.
3. From then on, `acc.SignIn` with the right password returns `ErrTwoStepRequired` instead of signing in, and remembers the sign-in for ten minutes in a cookie of its own. The app redirects to a page with the `TwoStepCode` block, whose action calls `acc.CompleteSignIn(ctx, code)` with a code from the app or a recovery code. A reset link asks for the code too.
4. `acc.TurnOffTwoStep(ctx, code)` turns it off, with a current code or a recovery code.

Each code works once, within 30 seconds either side of now; each recovery code works once and is stored hashed. `User.TwoStep` says whether it's on. Passkeys don't ask for a code: a passkey is two steps already, the device and its screen lock.

The `TwoStep` block shows all of this on an account page: the QR code while setting up, the recovery codes once, and the controls to turn it off or get new codes.

## Sign in with GitHub, Google or OpenID Connect

`plugins/oauth` signs users in to accounts when given `Accounts` in place of `OnLogin`:

```go
oauth.New(oauth.Options{
	Secret:    secret,
	Providers: []*oauth.Provider{oauth.GitHub(id, secret), oauth.Google(id, secret)},
	Accounts:  acc,
})
```

A provider's sign-in finds its user like this:

1. **Signed in already:** it connects the provider to the current user. This is how a "Connect GitHub" button on the account page works: link to `/auth/github/login?next=/account`.
2. **Connected before:** it signs in that user, by the provider's id for them, whatever the email says now.
3. **First time:** it joins the account with the same email if the provider verified the address, or creates an account without a password. An account with that email whose address the provider didn't verify isn't joined: the user signs in another way and connects the provider from there.

Accounts with two-step sign-in still ask for the code: the provider counts as the first step. When a sign-in can't go ahead, the user goes to `ErrorURL` with `?error=unverified`, `taken`, `no_email` or `limited`, and the `SignIn` block explains each.

`acc.Identities(ctx)` lists a user's connected providers, and `acc.Unlink(ctx, provider)` disconnects one, unless it's the only way left to sign in (`ErrLastSignIn`). The `LinkedAccounts` block shows both, with Connect buttons for the others. In your own code, `acc.SignInWith(ctx, accounts.Identity{…})` does the same for any provider.

## Changing the email address

`acc.RequestEmailChange(ctx, newEmail)` emails a confirmation link to the new address. The account keeps its old address until the link is opened at `VerifyPath`, where `acc.VerifyEmail` confirms it, so a typo can't lock anyone out. The old address then gets a notice, and links sent to it stop working. A new address that already has an account fails with `ErrEmailTaken`.

## Rate limits

Attempts are limited per account and per client IP, so passwords and codes can't be guessed and emails can't be sent in bulk:

| Attempt | Per account | Per IP |
|---|---|---|
| Signing in, and the current password when changing it | 10 per 15 minutes | 50 per 15 minutes |
| Signing up | | 20 per hour |
| Verification, reset and email change emails | 5 per hour | 30 per hour |
| Passkey sign-ins | | 50 per 15 minutes |
| Two-step codes | 6 per 15 minutes | 50 per 15 minutes |

Past a limit, the method returns `ErrTooManyAttempts`, which `FormError` shows as "Too many attempts". Signing in clears the account's count. The counts are kept in memory, per process. Behind a proxy, set `kit.WithTrustedProxies` so the client's own IP is counted, not the proxy's. `NoRateLimits` turns the limits off.

## Devices

Sessions live in the database, so `acc.Sessions(ctx)` lists the devices a user is signed in on: a description such as "Firefox on macOS", the IP address, and when it was last used. The current one comes first. `acc.Revoke(ctx, id)` signs one out and `acc.SignOutOthers(ctx)` all but this one. The cookie holds a random token and the database only its SHA-256 hash, so a copy of the database can't sign anyone in.

## Passkeys

Passkeys sign users in with a fingerprint, face or screen lock: nothing to remember or type, and nothing a phishing site can use. They're WebAuthn discoverable credentials, so signing in needs no email.

- **Adding one:** a signed-in user clicks "Add a passkey" in the `Passkeys` block. In code, `registerPasskey()` from `@ternkit/tern` does the same.
- **Signing in:** the `SignIn` block's `passkey` prop adds a "Sign in with a passkey" button, which calls `signInWithPasskey()`. Both helpers take `{ path }` if `PasskeyPath` isn't the default.
- **Managing them:** `acc.Passkeys(ctx)` lists them, and `acc.RemovePasskey(ctx, id)` removes one.

The server checks every answer: the challenge (each works once, for five minutes), the origin, the site's hash, that the user was present, and the signature, with ES256, EdDSA or RS256 keys. A signature counter that doesn't go up means a cloned key, and the sign-in fails. Attestation isn't asked for: a passkey proves it holds its key, not what device it is.

An account can sign in with passkeys only: create it with `acc.SignUp(ctx, email, name, "")`, then have the user add a passkey. `User.HasPassword` says whether it has a password.

Passkeys are tied to the site's host name. Set `BaseURL` to the address users visit; browsers allow passkeys on `localhost` over plain HTTP for development.

## Teams

Teams are groups of users who share the app's data: a company, a household, a class. A user can be in several and works in one at a time, the current team, which each session remembers.

```go
func LayoutLoad(ctx *kit.Ctx) (Layout, error) {
	m, err := acc.RequireTeam(ctx) // signed in, and in a team
	if err != nil {
		return Layout{}, err
	}
	projects, err := listProjects(ctx, m.Team.ID) // the app's rows have a team_id
	...
}
```

- `accounts.CurrentTeam(ctx)` is the current `*Membership` (its `Team` and the user's `Role`), or nil. `acc.RequireTeam(ctx)` is the same, or a redirect: to sign in, or to `NewTeamPath` to make a team.
- `acc.CreateTeam(ctx, name)` makes a team with the user as its owner and switches to it. `acc.Teams(ctx)` lists the user's teams, and `acc.SwitchTeam(ctx, id)` changes the current one.
- `acc.Members(ctx)` lists the current team's members. `acc.SetRole(ctx, userID, role)` and `acc.RemoveMember(ctx, userID)` change them, and `acc.LeaveTeam(ctx)` takes the user out.
- `acc.RenameTeam(ctx, name)` and `acc.DeleteTeam(ctx)` change the team itself. `OnDeleteTeam` deletes the app's data of it first.

### Roles

| Role | Can |
|---|---|
| owner | everything: make and remove owners, delete the team |
| admin | invite, remove and change members other than owners, rename the team |
| member | use the app |

A team always has an owner: the last one can't leave, be removed or be demoted (`ErrLastOwner`), and their account can't be deleted until someone else owns the team. Roles of the app's own, from `TeamRoles`, manage nothing in accounts. Check what any role may do with [`rbac`](guide.md#roles-and-permissions):

```go
var policy = rbac.New(map[string][]string{
	"owner":  {"*"},
	"admin":  {"projects:*", "billing:read"},
	"member": {"projects:read", "projects:write"},
	"viewer": {"projects:read"},
}, accounts.TeamRoles)
```

### Invitations

`acc.Invite(ctx, email, role)` emails a link to `InvitePath`, valid for a week. The page's load shows it with `acc.InvitationFor(ctx, token)` (the team's name, who invited, the role), and its action calls `acc.AcceptInvitation(ctx, token)` for a signed-in user. Only the invited address can accept (`ErrWrongInvitee`); send signed-out visitors to sign up or in with `?next=` back to the invitation. Accepting verifies the email address. `acc.Invitations(ctx)` lists the pending ones and `acc.CancelInvitation(ctx, id)` withdraws one.

Owners and admins invite; inviting an owner takes an owner. `FormError` turns each of these errors into a message for the form.

### The screens

Tern UI has a block for each: `NewTeam`, `TeamSwitcher`, `TeamMembers`, `TeamInvitations`, `TeamSettings`, `AcceptInvitation` and `ActivityLog` ([docs/ui.md](ui.md)). The full app that `tern new` makes (the saas template) uses all of them: everyone gets a team at sign-up, projects belong to the team, and the team's settings are at `/settings/team`.

```svelte
<TeamMembers members={data.members} me={data.me} role={data.team.role} roleAction="?/role" removeAction="?/remove" leaveAction="?/leave" {form} />
<TeamInvitations invitations={data.invitations} role={data.team.role} inviteAction="?/invite" cancelAction="?/cancel" {form} />
```

Several blocks on one page share its `form` prop. Each block's forms post a hidden `tern_form` field naming the block, and each block shows only its own errors. Return `{ invited: email }` from the invite action to show "Invitation sent".

## The audit log

accounts records what happens to accounts and teams: sign-ins and failed ones, sign-outs, password, email and two-step changes, passkeys, linked providers, sessions signed out, and teams created, renamed and deleted, with members invited, joined, changed, removed and leaving. Each entry has the time, who did it, the team, what it happened to, the IP and the device.

- `acc.Activity(ctx, before)` is the user's own log, for an account page: "was that sign-in me?". Failed sign-ins to their account are in it.
- `acc.TeamActivity(ctx, before)` is the current team's, for its owners and admins.
- `acc.AuditEntries(ctx, accounts.AuditFilter{...})` filters by user, team and action prefix, with no checks of who asks, for support tools behind checks of their own.
- `acc.Audit(ctx, accounts.Event{Action: "project.deleted", TeamID: id, Target: name})` adds the app's own events.

Lists are newest first, 50 at a time; pass the last entry's `ID` as `before` for the next page. Entries older than `AuditFor` are deleted as new ones come in.
