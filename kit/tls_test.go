package kit

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

type spyPlugin struct {
	mu     sync.Mutex
	hellos []string // ALPN offered
	seen   bytes.Buffer
}

func (*spyPlugin) Name() string { return "spy" }

func (p *spyPlugin) Setup(s *Setup) error {
	s.OnClientHello(func(h *tls.ClientHelloInfo) {
		p.mu.Lock()
		p.hellos = append(p.hellos, strings.Join(h.SupportedProtos, ","))
		p.mu.Unlock()
	})
	s.WrapConn(func(c net.Conn) net.Conn { return &spyConn{Conn: c, p: p} })
	s.ConnContext(func(ctx context.Context, c net.Conn) context.Context {
		if _, ok := c.(*spyConn); !ok {
			panic("ConnContext didn't get the plugin's wrapper")
		}
		return context.WithValue(ctx, spyKey{}, "conn")
	})
	return nil
}

type spyKey struct{}

type spyConn struct {
	net.Conn
	p *spyPlugin
}

func (c *spyConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.p.mu.Lock()
	c.p.seen.Write(b[:n])
	c.p.mu.Unlock()
	return n, err
}

func TestServeTLS(t *testing.T) {
	cert := selfSigned(t)
	spy := &spyPlugin{}
	m := testManifest()
	m.Routes = append(m.Routes, RouteDef{ID: "/proto", Endpoint: map[string]HandlerFunc{"GET": func(c *Ctx) error {
		secure := c.Request.TLS != nil && c.Request.TLS.HandshakeComplete
		v, _ := c.Value(spyKey{}).(string)
		return c.Text(http.StatusOK, c.Request.Proto+" "+map[bool]string{true: "tls", false: "plain"}[secure]+" "+v)
	}}})
	a, err := New(m, WithTLS(&tls.Config{Certificates: []tls.Certificate{cert}}), WithPlugins(spy))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln, err := a.WrapListener(raw)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: a}
	a.ConfigureServer(srv)
	go func() { _ = srv.Serve(ln) }()
	defer func() { _ = srv.Close() }()

	pool := x509.NewCertPool()
	leaf, _ := x509.ParseCertificate(cert.Certificate[0])
	pool.AddCert(leaf)
	get := func(h2 bool) string {
		tr := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}, ForceAttemptHTTP2: h2}
		if !h2 {
			tr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		}
		defer tr.CloseIdleConnections()
		resp, err := (&http.Client{Transport: tr}).Get("https://" + raw.Addr().String() + "/proto")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}
	if got := get(false); got != "HTTP/1.1 tls conn" {
		t.Errorf("HTTP/1.1: %q", got)
	}
	if got := get(true); got != "HTTP/2.0 tls conn" {
		t.Errorf("HTTP/2: %q", got)
	}
	spy.mu.Lock()
	defer spy.mu.Unlock()
	// An HTTP/1-only Go client offers no ALPN.
	if len(spy.hellos) != 2 || spy.hellos[0] != "" || !strings.HasPrefix(spy.hellos[1], "h2") {
		t.Errorf("hellos %q", spy.hellos)
	}
	if s := spy.seen.String(); !strings.Contains(s, "GET /proto HTTP/1.1\r\n") || !strings.Contains(s, "PRI * HTTP/2.0") {
		t.Errorf("the wrapper didn't see plaintext: %q", s)
	}
}

func TestRedirectHTTPS(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "http://example.com:80/a?b=1", nil)
	w := &recorder{h: http.Header{}}
	redirectHTTPS(w, r)
	if w.code != http.StatusMovedPermanently || w.h.Get("Location") != "https://example.com/a?b=1" {
		t.Fatalf("%d %s", w.code, w.h.Get("Location"))
	}
}

type recorder struct {
	h    http.Header
	code int
}

func (r *recorder) Header() http.Header         { return r.h }
func (r *recorder) Write(b []byte) (int, error) { return len(b), nil }
func (r *recorder) WriteHeader(c int)           { r.code = c }
