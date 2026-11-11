# Accounts

Users, sessions and passkeys with the `accounts` package and Tern UI's account screens: sign up, sign in with a password or a passkey, confirm your email, reset a forgotten password, see the devices you're signed in on and sign any of them out.

Emails go to an outbox at `/outbox` in place of an email provider.

```bash
pnpm install
go tool tern dev
```

See [docs/accounts.md](../../docs/accounts.md).
