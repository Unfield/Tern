// Package signals tells people from bots and recognizes devices, from what
// the server sees of each request: its TLS ClientHello (JA4), its HTTP/2
// settings and the order of its headers, how its headers fit the browser
// its User-Agent claims, and, optionally, a client check in the browser
// with a proof of work. It is for an app's own security: blocking abuse,
// limiting rates per client, noticing a sign-in from a new device. It
// doesn't follow people across sites, and it keeps nothing.
//
//	sig := signals.New(signals.Options{Check: true})
//	ternApp(kit.WithPlugins(sig))
//
//	func Actions… {
//		if s := signals.Of(c); s.Verdict == signals.Bot { … }
//	}
//
// The TLS and HTTP/2 fingerprints need the App to terminate TLS itself
// (kit.WithAutoTLS, kit.WithTLS). Behind a proxy that terminates it, the
// proxy can pass its JA4 on in a header (Options.JA4Header); the rest of
// the signals work anywhere.
package signals

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Unfield/Tern/kit"
)

// Options configures the plugin.
type Options struct {
	// Check adds the client check to every page: a script that solves a
	// small proof of work and looks for signs of automation, once a day
	// per browser. Signals.Checked tells who passed.
	Check bool
	// Difficulty is the check's proof of work, in leading zero bits of a
	// SHA-256. Each bit doubles the work; the default, 16, takes a phone
	// a fraction of a second.
	Difficulty int
	// Secret signs the check's challenges and results. The default is
	// TERN_SECRET's; without either, a random one is made at start, and
	// browsers check again after a restart.
	Secret string

	// Block answers requests that change things (POST, PUT, PATCH,
	// DELETE) with 403 when their score is Block or more. 0 blocks
	// nothing; the app decides with Of.
	Block int

	// JA4Header names a header with the client's JA4 fingerprint, set by
	// a proxy that terminates TLS in front of the App, such as
	// Cloudflare's. It is read only from proxies the App trusts
	// (kit.WithTrustedProxies).
	JA4Header string

	// DeviceCookie names the cookie that identifies a device. The default
	// is "tern_device".
	DeviceCookie string
}

// Plugin computes the signals. Make it with New.
type Plugin struct {
	o      Options
	secret []byte

	mu     sync.Mutex
	hellos map[net.Conn]helloEntry // ClientHellos not yet claimed by their connection
	added  int
	used   map[string]time.Time // check challenges already redeemed
}

type helloEntry struct {
	info helloInfo
	at   time.Time
}

// New returns the plugin.
func New(o Options) *Plugin {
	if o.Difficulty <= 0 {
		o.Difficulty = 16
	}
	if o.DeviceCookie == "" {
		o.DeviceCookie = "tern_device"
	}
	secret := o.Secret
	if secret == "" {
		secret = os.Getenv("TERN_SECRET")
	}
	p := &Plugin{o: o, hellos: map[net.Conn]helloEntry{}, used: map[string]time.Time{}}
	if secret != "" {
		sum := sha256.Sum256([]byte("tern signals\x00" + secret))
		p.secret = sum[:]
	} else {
		p.secret = make([]byte, 32)
		_, _ = rand.Read(p.secret)
	}
	return p
}

// Name implements kit.Plugin.
func (*Plugin) Name() string { return "signals" }

type connKey struct{}

type stateKey struct{}

// reqState is a request's signals, computed when first asked for.
type reqState struct {
	p    *Plugin
	r    *http.Request
	conn *conn
	head head
	got  bool // head was found

	once sync.Once
	sig  *Signals
	dev  sync.Once
}

// Setup implements kit.Plugin.
func (p *Plugin) Setup(s *kit.Setup) error {
	s.OnClientHello(func(h *tls.ClientHelloInfo) {
		info := ja4(h)
		p.mu.Lock()
		p.hellos[h.Conn] = helloEntry{info, time.Now()}
		p.added++
		if p.added%256 == 0 || len(p.hellos) > 4096 {
			// Hellos of handshakes that failed are never claimed.
			cut := time.Now().Add(-time.Minute)
			for k, e := range p.hellos {
				if e.at.Before(cut) {
					delete(p.hellos, k)
				}
			}
		}
		p.mu.Unlock()
	})
	s.WrapConn(func(c net.Conn) net.Conn { return &conn{Conn: c, p: p} })
	s.ConnContext(func(ctx context.Context, c net.Conn) context.Context {
		for c != nil {
			if sc, ok := c.(*conn); ok {
				return context.WithValue(ctx, connKey{}, sc)
			}
			u, ok := c.(interface{ NetConn() net.Conn })
			if !ok {
				break
			}
			c = u.NetConn()
		}
		return ctx
	})
	s.Middleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			st := &reqState{p: p}
			if c, ok := r.Context().Value(connKey{}).(*conn); ok {
				st.conn = c
				st.head, st.got = c.claim(r.Method, r.RequestURI)
			}
			r = r.WithContext(context.WithValue(r.Context(), stateKey{}, st))
			st.r = r
			if p.o.Block > 0 && unsafe(r.Method) && !strings.HasPrefix(r.URL.Path, checkPrefix) {
				if sig := st.signals(); sig.Score >= p.o.Block {
					s.Logger().InfoContext(r.Context(), "signals: blocked a request", "path", r.URL.Path, "score", sig.Score, "reasons", sig.Reasons)
					http.Error(w, "This request looks automated and was blocked.", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	})
	if p.o.Check {
		p.setupCheck(s)
	}
	return nil
}

func unsafe(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	}
	return true
}

// takeHello moves the ClientHello of c's connection from the plugin to c.
func (c *conn) takeHello() *helloInfo {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.helloK {
		return c.hello
	}
	tc, ok := c.Conn.(*tls.Conn)
	if !ok {
		c.helloK = true
		return nil
	}
	if !tc.ConnectionState().HandshakeComplete {
		return nil // try again later
	}
	c.helloK = true
	raw := tc.NetConn()
	c.p.mu.Lock()
	if e, ok := c.p.hellos[raw]; ok {
		c.hello = &e.info
		delete(c.p.hellos, raw)
	}
	c.p.mu.Unlock()
	return c.hello
}

// Verdicts.
const (
	Human      = "human"
	Suspicious = "suspicious"
	Bot        = "bot"
)

// Signals are what the plugin found of a request.
type Signals struct {
	// Score is 0 for a request that looks like a person's browser, up to
	// 100 for one that is plainly automated.
	Score int `json:"score"`
	// Verdict is Human (score under 30), Suspicious (under 70) or Bot.
	Verdict string `json:"verdict"`
	// Reasons are codes for what raised or lowered the score, such as
	// "tool:curl" or "tls-no-grease"; docs/signals.md lists them.
	Reasons []string `json:"reasons,omitempty"`

	// Client is what the User-Agent claims.
	Client Client `json:"client"`
	// JA4 is the TLS client fingerprint, when known.
	JA4 string `json:"ja4,omitempty"`
	// HTTP2 is the HTTP/2 fingerprint (SETTINGS, WINDOW_UPDATE, PRIORITY
	// and pseudo-header order), for HTTP/2 requests to the App's own TLS.
	HTTP2 string `json:"http2,omitempty"`
	// Headers fingerprints the request's headers: JA4H's first two parts,
	// from the method, version, header count, language and the header
	// names in the order sent.
	Headers string `json:"headers,omitempty"`
	// Proto is the HTTP version.
	Proto string `json:"proto"`

	// Checked reports that this browser passed the client check
	// (Options.Check) within the day.
	Checked bool `json:"checked"`

	// Device identifies the browser, from a cookie, with Of; NewDevice
	// reports that the cookie was set by this request.
	Device    string `json:"device,omitempty"`
	NewDevice bool   `json:"newDevice,omitempty"`
}

// Of returns the signals of c's request, and its device: the device
// cookie is set if the browser has none. It returns nil if the plugin
// isn't installed.
func Of(c *kit.Ctx) *Signals {
	st, ok := c.Value(stateKey{}).(*reqState)
	if !ok {
		return nil
	}
	sig := st.signals()
	st.dev.Do(func() { st.p.device(c, sig) })
	return sig
}

// From returns the signals of r, without the device. It returns nil if
// the plugin isn't installed.
func From(r *http.Request) *Signals {
	st, ok := r.Context().Value(stateKey{}).(*reqState)
	if !ok {
		return nil
	}
	return st.signals()
}

// Key identifies the client of r for rate limits: its fingerprints and IP
// address, so clients that share an address are told apart and a client
// that hops addresses is not. Without the plugin it is kit.ClientIP.
//
//	ratelimit.New(ratelimit.KeyFunc(signals.Key))
func Key(r *http.Request) string {
	ip := kit.ClientIP(r)
	sig := From(r)
	if sig == nil || sig.JA4 == "" && sig.HTTP2 == "" && sig.Headers == "" {
		return ip
	}
	sum := sha256.Sum256([]byte(sig.JA4 + "|" + sig.HTTP2 + "|" + sig.Headers + "|" + r.UserAgent()))
	return ip + "/" + hex.EncodeToString(sum[:8])
}

func (st *reqState) signals() *Signals {
	st.once.Do(func() { st.sig = st.p.compute(st) })
	return st.sig
}

var ja4Re = regexp.MustCompile(`^[tqd][0-9s]{2}[di][0-9]{4}[0-9a-z]{2}_[0-9a-f]{12}_[0-9a-f]{12}$`)

func (p *Plugin) compute(st *reqState) *Signals {
	r := st.r
	sig := &Signals{Proto: r.Proto, Client: parseUA(r.UserAgent())}
	var hello *helloInfo
	if st.conn != nil {
		hello = st.conn.takeHello()
		st.conn.mu.Lock()
		sig.HTTP2 = st.conn.h2fp
		st.conn.mu.Unlock()
		if r.ProtoMajor != 2 {
			sig.HTTP2 = ""
		}
	}
	if hello != nil {
		sig.JA4 = hello.ja4
	} else if p.o.JA4Header != "" && kit.FromTrustedProxy(r) {
		if v := strings.TrimSpace(r.Header.Get(p.o.JA4Header)); ja4Re.MatchString(v) {
			sig.JA4 = v
		}
	}
	if st.got {
		sig.Headers = headersFingerprint(st.head, r)
	}
	sig.Checked, sig.Reasons = p.checked(r)
	score := p.score(sig, r, hello, st)
	sig.Score = min(max(score, 0), 100)
	switch {
	case sig.Score >= 70:
		sig.Verdict = Bot
	case sig.Score >= 30:
		sig.Verdict = Suspicious
	default:
		sig.Verdict = Human
	}
	return sig
}

// score adds up the reasons. Each rule fires only on what it can see.
func (p *Plugin) score(sig *Signals, r *http.Request, hello *helloInfo, st *reqState) int {
	score := 0
	add := func(n int, reason string) {
		score += n
		sig.Reasons = append(sig.Reasons, reason)
	}
	floor := func(n int, reason string) {
		score = max(score, n)
		sig.Reasons = append(sig.Reasons, reason)
	}
	// The check's findings came first, from the cookie.
	for _, reason := range sig.Reasons {
		if strings.HasPrefix(reason, "client:") {
			score += 60
		}
	}
	cl := sig.Client
	ua := r.UserAgent()
	if ua == "" {
		add(40, "no-user-agent")
	}
	if cl.Tool != "" {
		floor(90, "tool:"+cl.Tool)
	}
	if cl.Crawler != "" {
		floor(95, "crawler:"+cl.Crawler)
	}
	browser := cl.Browser != ""
	if ua != "" && cl.Tool == "" && cl.Crawler == "" && !browser {
		add(15, "unknown-client")
	}
	if browser {
		if r.Header.Get("Accept-Language") == "" {
			add(20, "no-accept-language")
		}
		if r.Header.Get("Accept") == "" {
			add(15, "no-accept")
		}
		// Browsers send these on secure origins. localhost is one, but not
		// every build sends client hints there.
		secure := kit.IsHTTPS(r)
		if secure && cl.Chromium() && cl.Version >= 90 && cl.Browser != "Samsung Internet" {
			switch hints := r.Header.Get("Sec-Ch-Ua"); {
			case hints == "":
				add(25, "no-client-hints")
			case !brandVersion(hints, cl.Version):
				add(25, "client-hints-mismatch")
			}
		}
		fetchMetadata := cl.Chromium() && cl.Version >= 80 || cl.Browser == "Firefox" && cl.Version >= 90 || cl.Browser == "Safari" && cl.Version >= 17
		if (secure || isLocalhost(r.Host)) && fetchMetadata && r.Header.Get("Sec-Fetch-Mode") == "" {
			add(20, "no-fetch-metadata")
		}
		if hello != nil {
			if cl.Chromium() && !hello.grease {
				add(35, "tls-no-grease")
			}
			if hello.maxTLS != 0 && hello.maxTLS < tls.VersionTLS13 {
				add(25, "tls-old-version")
			}
			if !slices.Contains(hello.alpn, "h2") {
				add(30, "tls-no-h2")
			}
		}
		if st.got && st.head.pseudo != "" {
			if want := pseudoOrders[family(cl)]; len(want) > 0 && !slices.Contains(want, st.head.pseudo) {
				add(35, "http2-mismatch")
			}
		}
	}
	if sig.Checked {
		score -= 30
		sig.Reasons = append(sig.Reasons, "checked")
	}
	return score
}

// pseudoOrders are the HTTP/2 pseudo-header orders each browser family
// sends.
var pseudoOrders = map[string][]string{
	"chromium": {"m,a,s,p"},
	"firefox":  {"m,p,a,s"},
	"safari":   {"m,s,p,a", "m,s,a,p"},
}

func family(c Client) string {
	switch {
	case c.Chromium():
		if c.OS == "iOS" {
			return "safari" // every iOS browser is WebKit
		}
		return "chromium"
	case c.Browser == "Firefox":
		if c.OS == "iOS" {
			return "safari"
		}
		return "firefox"
	case c.Browser == "Safari":
		return "safari"
	}
	return ""
}

var brandRe = regexp.MustCompile(`"([^"]*)";\s*v="(\d+)"`)

// brandVersion reports whether Sec-CH-UA lists a brand at version v.
func brandVersion(hints string, v int) bool {
	for _, m := range brandRe.FindAllStringSubmatch(hints, -1) {
		if n, _ := strconv.Atoi(m[2]); n == v && !strings.Contains(strings.ToLower(m[1]), "brand") {
			return true
		}
	}
	return false
}

func isLocalhost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "127.0.0.1" || host == "::1" || host == "[::1]"
}

// headersFingerprint is JA4H_a and JA4H_b: method, version, cookie and
// referer present, header count and language; then a hash of the header
// names in the order sent. The cookie parts of JA4H are left out: they
// would tell users apart, not clients.
func headersFingerprint(h head, r *http.Request) string {
	method := strings.ToLower(h.method)
	if len(method) > 2 {
		method = method[:2]
	}
	version := "11"
	switch h.proto {
	case "HTTP/2.0":
		version = "20"
	case "HTTP/1.0":
		version = "10"
	}
	cookie, referer := "n", "n"
	var names []string
	for _, n := range h.names {
		switch strings.ToLower(n) {
		case "cookie":
			cookie = "c"
			continue
		case "referer":
			referer = "r"
			continue
		}
		names = append(names, n)
	}
	lang := strings.ToLower(strings.ReplaceAll(r.Header.Get("Accept-Language"), "-", ""))
	if i := strings.IndexAny(lang, ",;"); i >= 0 {
		lang = lang[:i]
	}
	lang = (lang + "0000")[:4]
	a := fmt.Sprintf("%s%s%s%s%02d%s", method, version, cookie, referer, min(len(names), 99), lang)
	return a + "_" + hash12(strings.Join(names, ","))
}

// device reads or sets the device cookie.
func (p *Plugin) device(c *kit.Ctx, sig *Signals) {
	if v, ok := c.Cookie(p.o.DeviceCookie); ok && validDevice(v) {
		sig.Device = v
		return
	}
	var b [16]byte
	_, _ = rand.Read(b[:])
	sig.Device = base64.RawURLEncoding.EncodeToString(b[:])
	sig.NewDevice = true
	c.SetCookie(&http.Cookie{
		Name:     p.o.DeviceCookie,
		Value:    sig.Device,
		Path:     "/",
		MaxAge:   400 * 24 * 3600, // browsers cap cookies at 400 days
		HttpOnly: true,
		Secure:   kit.IsHTTPS(c.Request),
		SameSite: http.SameSiteLaxMode,
	})
}

func validDevice(v string) bool {
	if len(v) != 22 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(v)
	return err == nil
}
