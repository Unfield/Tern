# 0015. Security signals

- Status: Accepted
- Date: 2026-11-11

## Context

Apps need to tell people from scripts, to keep bots out of sign-ups, contact forms and sign-in attempts, and to recognize devices, so users hear about sign-ins from new ones. The good commercial bot detection is expensive and runs in someone else's cloud. The open-source options mostly read the User-Agent, which a script sets to whatever it likes.

What a script can't easily fake is how it is built: its TLS library's ClientHello, its HTTP/2 framing, the order it sends headers in. Go's `net/http` keeps none of that: `http.Server` does the TLS handshake out of reach and hands handlers a header map.

## Decision

- **kit terminates TLS itself** when it serves HTTPS (`WithTLS`, and `WithAutoTLS` with Let's Encrypt through `x/crypto/acme/autocert`). Its listener does the handshake and hands `http.Server` the decrypted connection. `http.Server` speaks HTTP/2 on it through `Protocols.SetUnencryptedHTTP2`, and kit gives each request its TLS state back in `ServeHTTP`. In exchange, plugins get three hooks: `Setup.WrapConn` sees a connection's bytes as the client sent them, `Setup.OnClientHello` sees the ClientHello, and `Setup.ConnContext` adds to its requests' context. The same TLS serving makes deploying to a bare server possible without a proxy.
- **`plugins/signals` computes the signals, lazily:** JA4 from the ClientHello, an HTTP/2 fingerprint from the client's first frames, and JA4H's header-order fingerprint from the request heads. It parses them from the bytes as they are read: HTTP/1 heads, and HTTP/2 frames with their header blocks decoded by `x/net/http2/hpack`. Each request claims the head with its method and target. Everything else costs nothing until a load calls `signals.Of`.
- **The score is a sum of contradictions,** each with a reason code. A Chromium User-Agent whose ClientHello has no GREASE values is not Chromium, nor is one whose pseudo-headers come in curl's order. The rules are written down, and the app sees the reasons. Nothing is learned from traffic or sent anywhere.
- **The client check is a proof of work plus three automation tells,** not a browser fingerprint. Its result is a signed cookie for a day. It proves the client ran a JavaScript engine and paid for its requests, without collecting what would identify a person.
- **Devices are a random cookie,** which accounts uses to notice sign-ins from new devices. It isn't derived from fingerprints, so it says nothing outside the app.

## Consequences

- Behind a proxy that terminates TLS, the TLS and HTTP/2 fingerprints are the proxy's. The plugin reads JA4 from a trusted proxy's header if one is configured, and the rest of the rules still work. `tern deploy` to a server can run the app on ports 80 and 443 directly.
- Requests over kit's TLS pass through one more `Read` wrapper, and HTTP/2 header blocks are decoded twice. That costs microseconds per request, and only with the plugin installed.
- The rules match today's browsers. A browser that changes how it builds requests can be scored as suspicious until the rules follow, so they only add to a score, and blocking is opt-in.
