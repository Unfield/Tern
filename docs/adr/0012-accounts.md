# 0012. User accounts

- Status: Accepted
- Date: 2026-11-11

## Context

Every app with users rebuilds the same things: sign-up, password hashing, sessions, email verification, password resets, "sign out my other devices", and now passkeys. Tern had the pieces (`plugins/sessions`, `mail`, `jobs`, `sqldb`) and a guide recipe that wires them together, but each app still wrote the security-sensitive parts itself. The recipe's cookie sessions can't be listed or revoked, which rules out a devices page.

## Decision

Add an `accounts` package: a plugin that keeps users, sessions, one-time tokens and passkeys in the app's database (SQLite or PostgreSQL), and Tern UI blocks for each screen.

- **Sessions in the database.** The cookie holds 32 random bytes; the table holds their SHA-256 hash, so a leaked database signs no one in. Sessions can be listed, described ("Firefox on macOS") and revoked. A session's expiry moves with use, written at most every five minutes so reads don't become writes.
- **argon2id** with OWASP's parameters for new hashes, from `golang.org/x/crypto`. bcrypt hashes verify, so users can be imported, and are rehashed at the next sign-in. A sign-in for an unknown email hashes a dummy password, so timing doesn't tell who has an account.
- **One table of one-time tokens** for verification links, reset links and passkey challenges: stored hashed, with a purpose and an expiry, deleted when used.
- **Passkeys without a WebAuthn library.** The server needs a small part of WebAuthn: a CBOR decoder for the authenticator's data, COSE keys for ES256, EdDSA and RS256, and the checks of the spec's verification steps. That's about 600 lines of Go, against a dependency larger than the package. Passkeys are discoverable credentials, so signing in needs no email. Attestation isn't requested: Tern doesn't decide which authenticators are allowed.
- **Texts are Tern's own** (`tern.accounts.*`), so emails and form errors are translated like everything else, and `accounts.FormError` turns the errors users cause into form errors.

## Consequences

- An app gets accounts by adding a plugin and a few pages, and [examples/accounts](../../examples/accounts) runs every flow in a browser test, passkeys included through Chromium's virtual authenticator.
- Each request of a signed-in user reads its session from the database. With SQLite that's a local read; with PostgreSQL it's a round trip, which an app with heavy traffic may want to cache.
- `plugins/sessions` stays, for apps that keep users elsewhere or need no server-side sessions.
- The WebAuthn code is Tern's to maintain. It's tested against a software authenticator in Go and against Chromium's in the browser.
