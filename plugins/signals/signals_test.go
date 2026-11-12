package signals

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"

	"github.com/Unfield/Tern/kit"
)

const chromeUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

func TestParseUA(t *testing.T) {
	for ua, want := range map[string]Client{
		chromeUA: {Browser: "Chrome", Version: 130, OS: "macOS"},
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36 Edg/130.0.0.0":           {Browser: "Edge", Version: 130, OS: "Windows"},
		"Mozilla/5.0 (X11; Linux x86_64; rv:132.0) Gecko/20100101 Firefox/132.0":                                                                  {Browser: "Firefox", Version: 132, OS: "Linux"},
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.1 Mobile/15E148 Safari/604.1": {Browser: "Safari", Version: 18, OS: "iOS", Mobile: true},
		"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Mobile Safari/537.36":                   {Browser: "Chrome", Version: 130, OS: "Android", Mobile: true},
		"curl/8.7.1":             {Tool: "curl"},
		"python-requests/2.32.3": {Tool: "python-requests"},
		"Go-http-client/2.0":     {Tool: "Go-http-client"},
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/130.0.0.0 Safari/537.36": {Tool: "HeadlessChrome", OS: "Linux"},
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)":                                      {Crawler: "Googlebot"},
	} {
		if got := parseUA(ua); got != want {
			t.Errorf("%s:\n got %+v\nwant %+v", ua, got, want)
		}
	}
}

func TestCaptureH1(t *testing.T) {
	c := &conn{}
	in := "GET /a HTTP/1.1\r\nHost: x\r\nUser-Agent: u\r\nAccept: */*\r\n\r\n" +
		"POST /b HTTP/1.1\r\nHost: x\r\nContent-Length: 5\r\nContent-Type: text/plain\r\n\r\nhello" +
		"GET /c HTTP/1.1\r\nhost: x\r\nCookie: a=b\r\n\r\n"
	// Fed a byte at a time, as reads may split anywhere.
	for i := range len(in) {
		c.feed([]byte{in[i]})
	}
	for _, want := range []struct{ method, target, names string }{
		{"GET", "/a", "Host,User-Agent,Accept"},
		{"POST", "/b", "Host,Content-Length,Content-Type"},
		{"GET", "/c", "host,Cookie"},
	} {
		h, ok := c.claim(want.method, want.target)
		if !ok || strings.Join(h.names, ",") != want.names || h.proto != "HTTP/1.1" {
			t.Errorf("%s %s: %+v %v", want.method, want.target, h, ok)
		}
	}
	// A chunked body ends the watching.
	c = &conn{}
	c.feed([]byte("POST /d HTTP/1.1\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\nGET /e HTTP/1.1\r\n\r\n"))
	if _, ok := c.claim("POST", "/d"); !ok {
		t.Error("chunked request head lost")
	}
	if _, ok := c.claim("GET", "/e"); ok || c.mode != modeOff {
		t.Error("parsed past a chunked body")
	}
}

func TestCaptureH2(t *testing.T) {
	var buf bytes.Buffer
	buf.WriteString(http2.ClientPreface)
	fr := http2.NewFramer(&buf, nil)
	_ = fr.WriteSettings(http2.Setting{ID: 1, Val: 65536}, http2.Setting{ID: 2, Val: 0}, http2.Setting{ID: 4, Val: 6291456}, http2.Setting{ID: 6, Val: 262144})
	_ = fr.WriteWindowUpdate(0, 15663105)
	var hb bytes.Buffer
	enc := hpack.NewEncoder(&hb)
	for _, f := range [][2]string{{":method", "GET"}, {":authority", "x"}, {":scheme", "https"}, {":path", "/a"}, {"sec-ch-ua", "x"}, {"user-agent", "u"}, {"accept", "*/*"}} {
		_ = enc.WriteField(hpack.HeaderField{Name: f[0], Value: f[1]})
	}
	_ = fr.WriteHeaders(http2.HeadersFrameParam{StreamID: 1, BlockFragment: hb.Bytes(), EndHeaders: true, EndStream: true})
	_ = fr.WriteData(1, false, bytes.Repeat([]byte("x"), 1000))
	hb.Reset()
	for _, f := range [][2]string{{":method", "POST"}, {":authority", "x"}, {":scheme", "https"}, {":path", "/b"}, {"user-agent", "u"}} {
		_ = enc.WriteField(hpack.HeaderField{Name: f[0], Value: f[1]}) // uses the dynamic table
	}
	half := hb.Len() / 2
	_ = fr.WriteHeaders(http2.HeadersFrameParam{StreamID: 3, BlockFragment: hb.Bytes()[:half]})
	_ = fr.WriteContinuation(3, true, hb.Bytes()[half:])

	c := &conn{}
	data := buf.Bytes()
	for len(data) > 0 { // in uneven reads
		n := min(len(data), 7)
		c.feed(data[:n])
		data = data[n:]
	}
	if c.h2fp != "1:65536;2:0;4:6291456;6:262144|15663105|0|m,a,s,p" {
		t.Errorf("fingerprint %q", c.h2fp)
	}
	a, ok := c.claim("GET", "/a")
	if !ok || strings.Join(a.names, ",") != "sec-ch-ua,user-agent,accept" || a.pseudo != "m,a,s,p" {
		t.Errorf("first: %+v", a)
	}
	if b, ok := c.claim("POST", "/b"); !ok || strings.Join(b.names, ",") != "user-agent" {
		t.Errorf("second: %+v %v", b, ok)
	}
}

func TestHeadersFingerprint(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Accept-Language", "en-US,en;q=0.9")
	h := head{method: "GET", proto: "HTTP/1.1", names: []string{"Host", "User-Agent", "Cookie", "Accept-Language", "Referer"}}
	got := headersFingerprint(h, r)
	if want := "ge11cr03enus_" + hash12("Host,User-Agent,Accept-Language"); got != want {
		t.Fatalf("%s, want %s", got, want)
	}
}

// browserRequest is a request with the headers Chrome sends.
func browserRequest() *http.Request {
	r := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
	r.TLS = &tls.ConnectionState{}
	r.Header.Set("User-Agent", chromeUA)
	r.Header.Set("Accept", "text/html")
	r.Header.Set("Accept-Language", "en-US")
	r.Header.Set("Sec-Ch-Ua", `"Chromium";v="130", "Google Chrome";v="130", "Not?A_Brand";v="99"`)
	r.Header.Set("Sec-Fetch-Mode", "navigate")
	return r
}

func scoreOf(p *Plugin, r *http.Request, hello *helloInfo, h *head) *Signals {
	st := &reqState{p: p, r: r}
	if h != nil {
		st.head, st.got = *h, true
	}
	if hello != nil {
		st.conn = &conn{hello: hello, helloK: true}
	}
	return p.compute(st)
}

func TestScore(t *testing.T) {
	p := New(Options{Secret: "s"})
	chromeHello := &helloInfo{ja4: "t13d1516h2_8daaf6152771_e5627efa2ab1", grease: true, alpn: []string{"h2", "http/1.1"}, maxTLS: tls.VersionTLS13}
	if s := scoreOf(p, browserRequest(), chromeHello, &head{method: "GET", proto: "HTTP/2.0", pseudo: "m,a,s,p"}); s.Score != 0 || s.Verdict != Human || s.JA4 == "" {
		t.Errorf("a real Chrome: %+v", s)
	}

	r := browserRequest()
	r.Header.Del("Sec-Ch-Ua")
	r.Header.Del("Accept-Language")
	if s := scoreOf(p, r, nil, nil); s.Score != 45 || s.Verdict != Suspicious || !hasAll(s.Reasons, "no-client-hints", "no-accept-language") {
		t.Errorf("missing headers: %+v", s)
	}

	// Chrome's User-Agent over a TLS library that doesn't GREASE, without
	// h2, and Go's pseudo-header order: a script dressed as Chrome.
	fake := &helloInfo{alpn: []string{"http/1.1"}, maxTLS: tls.VersionTLS13}
	if s := scoreOf(p, browserRequest(), fake, &head{method: "GET", proto: "HTTP/2.0", pseudo: "m,p,s,a"}); s.Verdict != Bot || !hasAll(s.Reasons, "tls-no-grease", "tls-no-h2", "http2-mismatch") {
		t.Errorf("a disguised script: %+v", s)
	}

	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("User-Agent", "curl/8.7.1")
	if s := scoreOf(p, r, nil, nil); s.Score != 90 || s.Verdict != Bot || s.Client.Tool != "curl" {
		t.Errorf("curl: %+v", s)
	}
}

func hasAll(have []string, want ...string) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			found = found || h == w
		}
		if !found {
			return false
		}
	}
	return true
}

func TestCheck(t *testing.T) {
	p := New(Options{Secret: "s", Difficulty: 8})
	rec := httptest.NewRecorder()
	p.challenge(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	var ch struct {
		Challenge  string `json:"challenge"`
		Difficulty int    `json:"difficulty"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &ch); err != nil || ch.Difficulty != 8 {
		t.Fatal(rec.Body.String(), err)
	}
	nonce := 0
	for zeroBits(sha256.Sum256([]byte(ch.Challenge+":"+strconv.Itoa(nonce)))) < 8 {
		nonce++
	}
	post := func(rep checkReport) *httptest.ResponseRecorder {
		b, _ := json.Marshal(rep)
		w := httptest.NewRecorder()
		p.verify(w, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(b)))
		return w
	}
	// A nonce that doesn't solve it; posting one that happens to would
	// redeem the challenge.
	wrong := nonce + 1
	for zeroBits(sha256.Sum256([]byte(ch.Challenge+":"+strconv.Itoa(wrong)))) >= 8 {
		wrong++
	}
	if w := post(checkReport{Challenge: ch.Challenge, Nonce: strconv.Itoa(wrong)}); w.Code != http.StatusForbidden {
		t.Errorf("a wrong solution: %d", w.Code)
	}
	w := post(checkReport{Challenge: ch.Challenge, Nonce: strconv.Itoa(nonce)})
	if w.Code != http.StatusNoContent {
		t.Fatalf("solution: %d %s", w.Code, w.Body.String())
	}
	if w := post(checkReport{Challenge: ch.Challenge, Nonce: strconv.Itoa(nonce)}); w.Code != http.StatusForbidden {
		t.Errorf("reused: %d", w.Code)
	}
	r := browserRequest()
	for _, ck := range w.Result().Cookies() {
		r.AddCookie(ck)
	}
	if s := scoreOf(p, r, nil, nil); !s.Checked || s.Score != 0 || !hasAll(s.Reasons, "checked") {
		t.Errorf("after the check: %+v", s)
	}

	// A browser driven by WebDriver passes the work but is flagged.
	rec = httptest.NewRecorder()
	p.challenge(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	_ = json.Unmarshal(rec.Body.Bytes(), &ch)
	nonce = 0
	for zeroBits(sha256.Sum256([]byte(ch.Challenge+":"+strconv.Itoa(nonce)))) < 8 {
		nonce++
	}
	w = post(checkReport{Challenge: ch.Challenge, Nonce: strconv.Itoa(nonce), Webdriver: true})
	r = browserRequest()
	for _, ck := range w.Result().Cookies() {
		r.AddCookie(ck)
	}
	if s := scoreOf(p, r, nil, nil); s.Checked || s.Verdict != Suspicious || !hasAll(s.Reasons, "client:webdriver") {
		t.Errorf("webdriver: %+v", s)
	}
	// A forged cookie counts for nothing.
	r = browserRequest()
	r.AddCookie(&http.Cookie{Name: checkCookie, Value: "9999999999..AAAA"})
	if s := scoreOf(p, r, nil, nil); s.Checked {
		t.Error("forged cookie accepted")
	}
}

func selfSigned(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

// End to end: the App terminates TLS, and a Go client's request gets its
// JA4, HTTP/2 fingerprint and header order, and a device.
func TestInApp(t *testing.T) {
	cert, pool := selfSigned(t)
	p := New(Options{Secret: "s", Block: 70})
	var got *Signals
	m := &kit.Manifest{AppHTML: "<html><head>%tern.head%</head><body>%tern.body%</body></html>", Routes: []kit.RouteDef{
		{ID: "/sig", Endpoint: map[string]kit.HandlerFunc{
			"GET": func(c *kit.Ctx) error { got = Of(c); return c.Text(http.StatusOK, "ok") },
			"POST": func(c *kit.Ctx) error {
				return c.Text(http.StatusOK, "posted")
			},
		}},
	}}
	a, err := kit.New(m, kit.WithTLS(&tls.Config{Certificates: []tls.Certificate{cert}}), kit.WithPlugins(p))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := net.Listen("tcp", "127.0.0.1:0")
	ln, err := a.WrapListener(raw)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: a}
	a.ConfigureServer(srv)
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}, ForceAttemptHTTP2: true}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr}
	resp, err := client.Get("https://" + raw.Addr().String() + "/sig")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if got == nil {
		t.Fatal("no signals")
	}
	if got.Proto != "HTTP/2.0" || !strings.HasPrefix(got.JA4, "t13i") /* an IP address: no SNI */ || !strings.HasSuffix(got.HTTP2, "|a,m,p,s") /* Go's order */ || !strings.HasPrefix(got.Headers, "ge20nn") {
		t.Errorf("fingerprints %+v", got)
	}
	if got.Client.Tool != "Go-http-client" || got.Verdict != Bot || got.Device == "" || !got.NewDevice {
		t.Errorf("verdict %+v", got)
	}
	if !strings.Contains(resp.Header.Get("Set-Cookie"), "tern_device=") {
		t.Errorf("no device cookie: %v", resp.Header)
	}
	// Block stops its POST.
	resp, err = client.Post("https://"+raw.Addr().String()+"/sig", "text/plain", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("POST from a bot: %d", resp.StatusCode)
	}
}
