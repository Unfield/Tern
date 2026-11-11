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

`Migrate` creates four tables if they don't exist: `tern_users`, `tern_sessions`, `tern_tokens` (email links and passkey challenges) and `tern_passkeys`. Keep `acc` where your loads and actions can reach it, such as a package variable.

| Option | Default | |
|---|---|---|
| `Send` | none | delivers emails. `SendWith(mailer, queue)` sends them with the `mail` package through a `jobs` queue, so they're retried. Without it, `SendVerification` and `RequestPasswordReset` return `ErrNoEmail`. |
| `BaseURL` | the request's | the app's address for links in emails and for passkeys, such as `https://example.com`. Set it in production. |
| `SignInPath` | `/login` | where `Require` sends signed-out users |
| `VerifyPath`, `ResetPath` | `/verify-email`, `/reset-password` | the pages the emails link to, with `?token=` |
| `Cookie` | `tern_session` | the session cookie's name |
| `SessionLength` | 30 days | how long a "keep me signed in" session lasts without use. Other sessions last a day. Sessions in use stay signed in. |
| `PasskeyPath` | `/_tern/passkeys` | where the passkey endpoints are |

## Who is signed in

- `accounts.Current(ctx)` is the signed-in `*accounts.User`, or nil.
- `acc.Require(ctx)` is the user, or a redirect to the sign-in page with `?next=` set to come back. Return its error from a load or action.

A layout load is a good place to guard a group of pages:

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
| `/login` | `SignIn passkey remember forgotHref="/forgot-password"` | `acc.SignIn(ctx, email, password, remember)` |
| `/forgot-password` | `ForgotPassword` | `acc.RequestPasswordReset(ctx, email)`; return `{ sent: true }` |
| `/reset-password` | `ResetPassword token={data.token}` | `acc.ResetPassword(ctx, token, password)` |
| `/verify-email` | none: the load calls `acc.VerifyEmail(ctx, token)` | |
| `/account` | `Devices`, `Passkeys` | `acc.Revoke`, `acc.SignOutOthers`, `acc.RemovePasskey`, `acc.ChangePassword`, `acc.SetName`, `acc.DeleteAccount` |
| sign out | a form posting to an action | `acc.SignOut(ctx)` |

## Passwords

Passwords are hashed with argon2id with the parameters OWASP recommends (19 MiB, 2 passes). bcrypt hashes from another system also work: import them into `tern_users.password` and they're rehashed with argon2id when the user next signs in.

Signing in takes the same time whether or not the email has an account, and its error doesn't say which part was wrong. Add `plugins/ratelimit` to slow down guessing.

Changing or resetting a password signs out the user's other devices.

## Email verification and password resets

`acc.SendVerification(ctx)` emails the signed-in user a link to `VerifyPath`; it's valid for two days. `acc.RequestPasswordReset(ctx, email)` emails a link to `ResetPath`, valid for an hour, and does nothing for an email without an account, so the page can't be used to find out who has one. Each link works once.

The emails' texts are Tern's own (`tern.accounts.verify.*`, `tern.accounts.reset.*`), in the request's language: translate them in `src/locales` like the app's messages. To send your own emails instead, pass a `Send` function: it gets an `accounts.Email` with the recipient, the link, a subject, and text and HTML bodies.

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
