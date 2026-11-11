# Security signals

`plugins/signals` tells people from bots, and recognizes devices, from what the server sees of each request. It's for an app's own security: blocking abuse of sign-up and contact forms, limiting rates per client instead of per address, and telling users when their account is signed in to from a new device. It doesn't follow people across sites, it sends nothing anywhere, and it keeps nothing but a device cookie in the browser.

```go
sig := signals.New(signals.Options{Check: true})
server, err := ternApp(
	kit.WithAutoTLS(kit.AutoTLS{Domains: []string{"example.com"}}), // for the TLS and HTTP/2 fingerprints
	kit.WithPlugins(sig),
)
```

In a load, action or endpoint:

```go
s := signals.Of(c)
if s.Verdict == signals.Bot {
	return kit.Error(http.StatusForbidden, "Sign-ups from scripts aren't allowed.")
}
```

## What it looks at

| Signal | What it is | Where it works |
|--------|------------|----------------|
| `JA4` | The [JA4](https://github.com/FoxIO-LLC/ja4) fingerprint of the TLS ClientHello: TLS version, cipher suites, extensions and signature algorithms. A script's TLS library looks different from a browser's whatever User-Agent it sends. | The App terminates TLS (`kit.WithAutoTLS`, `kit.WithTLS`), or a trusted proxy passes it on (`Options.JA4Header`). |
| `HTTP2` | The HTTP/2 fingerprint: the client's SETTINGS, its connection window, PRIORITY frames, and the order of its pseudo-headers. | HTTP/2 to the App's own TLS. |
| `Headers` | JA4H's first two parts: method, version, whether there's a cookie and a referer, the header count, the language, and a hash of the header names in the order sent. | Anywhere. Behind a proxy, it's the proxy's order. |
| `Client` | What the User-Agent claims: browser, version, system, or the HTTP library or crawler it names. | Anywhere. |
| `Checked` | The browser passed the client check within the day. | With `Options.Check`. |
| `Device` | A random ID from a cookie the plugin sets, so the same browser is recognized later. | `signals.Of`, anywhere. |

Go's `net/http` keeps neither the ClientHello nor the order of headers. So kit terminates TLS itself when it serves HTTPS, and lets the plugin see a connection's bytes before they are parsed (`Setup.WrapConn`, `Setup.OnClientHello`).

## The score

Each rule that fires adds to `Score`, from 0 to 100, and its code is added to `Reasons`. `Verdict` is `human` under 30, `suspicious` under 70, and `bot` from 70.

| Reason | Score | When |
|--------|-------|------|
| `tool:NAME` | at least 90 | The User-Agent names an HTTP library or automation tool: curl, python-requests, Go-http-client, HeadlessChrome and others. |
| `crawler:NAME` | at least 95 | It names a crawler, such as Googlebot. Whether it is one isn't checked. |
| `no-user-agent` | +40 | There's no User-Agent. |
| `unknown-client` | +15 | The User-Agent is neither a known browser nor a known tool. |
| `no-accept-language`, `no-accept` | +20, +15 | A browser that sends no Accept-Language or Accept. Browsers always send them. |
| `no-client-hints`, `client-hints-mismatch` | +25 | Chrome, Edge or Opera 90 and later, over HTTPS, without Sec-CH-UA, or with a version that isn't the User-Agent's. |
| `no-fetch-metadata` | +20 | A browser that sends Sec-Fetch-Mode, but didn't. |
| `tls-no-grease` | +35 | A Chromium User-Agent whose ClientHello has no GREASE values. Chromium always sends them, and most TLS libraries don't. |
| `tls-old-version`, `tls-no-h2` | +25, +30 | A browser's ClientHello without TLS 1.3, or without HTTP/2. |
| `http2-mismatch` | +35 | The pseudo-header order isn't the one the claimed browser sends (`m,a,s,p` for Chromium, `m,p,a,s` for Firefox, `m,s,p,a` or `m,s,a,p` for Safari). |
| `client:webdriver`, `client:no-window-chrome`, `client:no-languages` | +60 each | The client check found signs of automation. |
| `checked` | −30 | The client check passed. |

The rules look for contradictions: a request that claims to be Chrome but isn't built like Chrome's. A script that sends Chrome's headers in Chrome's order over Chrome's TLS stack is hard to tell from Chrome, and so is a real browser driven by automation, which is what the client check is for. Treat `suspicious` as a reason to ask for more, such as a confirmation email, rather than to refuse.

`Options.Block` refuses requests that change things (POST, PUT, PATCH and DELETE) from clients at or above a score, with 403:

```go
signals.New(signals.Options{Check: true, Block: 70})
```

## The client check

With `Options.Check`, every page gets a small script, which runs once a day per browser, when the browser is idle:
1. It solves a proof of work: a SHA-256 with `Difficulty` leading zero bits, 16 by default. A phone takes a fraction of a second.
2. It reports three facts: whether `navigator.webdriver` is set, whether a desktop Chrome lacks `window.chrome`, and whether there are no languages.
3. The server checks the solution and sets a signed cookie for a day.

A solution is good once, within five minutes, and only from the address the challenge went to. The proof of work makes sending many requests cost something, and the cookie makes the check cheap for people.

The script is served from `/__tern/signals/check.js` with the page's CSP nonce. Set `Options.Secret`, or `TERN_SECRET`, so check cookies stay valid across restarts.

## Rate limits per client

`signals.Key` identifies a client by its fingerprints and address, for `plugins/ratelimit`. Clients behind one address, such as an office's, are told apart, and the same script doesn't get a fresh allowance on each address:

```go
ratelimit.New(ratelimit.Rate(10, time.Minute), ratelimit.KeyFunc(signals.Key))
```

## New devices

With the plugin installed, `accounts` can tell a user when their account is signed in to from a device it hadn't been signed in on before:

```go
accounts.New(db, accounts.Options{
	Send:             accounts.SendWith(mailer, queue),
	NotifyNewDevices: true,
	OnNewDevice: func(c *kit.Ctx, d accounts.NewDevice) {
		c.Logger().Info("new device", "user", d.User.ID, "device", d.Device)
	},
})
```

A device is a browser with the plugin's device cookie. The first device an account is used on isn't news, and neither is a device signed in on before. Clearing cookies makes a browser a new device.

## Behind a proxy

When a proxy or CDN terminates TLS, the App sees the proxy's TLS and headers, not the client's. The User-Agent rules, the client check and device IDs still work. A proxy that computes JA4 can pass it on in a header of your choosing. Cloudflare with Bot Management can, with a rule that adds `cf.bot_management.ja4` as a request header:

```go
kit.WithTrustedProxies("173.245.48.0/20" /* and the rest of the proxy's ranges */)
signals.New(signals.Options{JA4Header: "X-JA4"})
```

The header is read only from trusted proxies, since a client could set it to anything.
