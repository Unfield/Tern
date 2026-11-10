# 0008. OAuth 2.0 and OpenID Connect, as a client and as a provider

- Status: Accepted
- Date: 2026-11-10

## Context

Apps need two kinds of sign-in. One is "sign in with GitHub or Google". The other is being the identity provider for an organization's other apps. Both are security-critical, and most of the risk is in details: redirect validation, PKCE, token storage, and replay of refresh tokens.

## Decision

- **Two plugins.** `plugins/oauth` is the client side and `plugins/oidc` is the provider. Neither stores users: the app maps identities to users in `OnLogin`, and authenticates users for the provider in `Authenticate`. Tern stays out of the user model.
- **Well-reviewed libraries for the cryptography.**
  - `golang.org/x/oauth2` handles the client's code exchange and PKCE.
  - `github.com/coreos/go-oidc` handles discovery and ID-token verification.
  - `github.com/go-jose/go-jose/v4` signs and verifies JWTs.
  - Tern implements the protocol logic. Its tests use go-oidc to validate the provider end to end.
- **The client** always uses PKCE (S256) and `state`, plus a nonce for OpenID Connect. These are kept in an AES-GCM-sealed cookie scoped to the provider's callback path and valid for ten minutes. `next` must be a local path.
- **The provider** follows OAuth 2.0 Security Best Current Practice (RFC 9700):
  - It supports only the authorization code flow, with no implicit flow.
  - PKCE is required for public clients, and only S256 is accepted.
  - Redirect URIs must match exactly. Errors about the client or redirect URI are shown, never redirected.
  - Authorization responses include `iss` (RFC 9207).
  - Refresh tokens rotate. Reusing one revokes its whole family.
  - Codes and refresh tokens are stored only as SHA-256 hashes, and codes are single-use.
- **Access tokens are signed JWTs** (RFC 9068), so resource servers can verify them offline. Revoked access tokens are kept in a deny list until they expire, which `Bearer` and introspection check.
- **The consent screen** is rendered by the plugin with its own strict CSP: no scripts, and no framing.
- **Cross-site protection exemptions.** The token, userinfo, revocation and introspection endpoints are exempt from kit's cross-site request protection (`Setup.AllowCrossOrigin`) and send CORS headers. They are authenticated by client credentials or bearer tokens, never cookies.

## Consequences

- The provider supports what most deployments need. Some features are left out for now:
  - dynamic client registration
  - `prompt=login` and `max_age` re-authentication
  - `request` objects
  - logout endpoints
  - key rotation with several published keys
- These can be added without changing the API.
- Clients are configured in code. Apps that manage clients at runtime can build `Options.Clients` from their database at startup.
