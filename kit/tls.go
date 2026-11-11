package kit

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// WithTLS serves HTTPS with cfg, which must have certificates
// (Certificates or GetCertificate). HTTP/2 is offered as well as HTTP/1.1.
//
// Without a port set (WithAddr, or PORT), HTTPS is served on :443.
func WithTLS(cfg *tls.Config) Option {
	return func(o *options) { o.tls = cfg }
}

// AutoTLS gets and renews certificates from Let's Encrypt, or another ACME
// service, for [WithAutoTLS].
type AutoTLS struct {
	// Domains are the host names to get certificates for. Requests for
	// other names get no certificate.
	Domains []string
	// Email is given to the ACME service for notices about the
	// certificates. Optional.
	Email string
	// Cache is the directory certificates are kept in across restarts.
	// The default is "tern-certs" in the working directory. Keep it: an
	// app that asks for new certificates on every start runs into the
	// service's rate limits.
	Cache string
	// DirectoryURL is the ACME service. The default is Let's Encrypt;
	// use https://acme-staging-v02.api.letsencrypt.org/directory to test.
	DirectoryURL string
	// HTTPAddr is where plain HTTP is served: the service's http-01
	// challenges, and redirects to HTTPS for everything else. The default
	// is ":80"; "-" serves no plain HTTP, leaving the tls-alpn-01
	// challenge, which needs port 443.
	HTTPAddr string
}

// WithAutoTLS serves HTTPS with certificates from Let's Encrypt for the
// domains in a, and redirects plain HTTP to it. The machine must be
// reachable at those domains on port 443, or 80.
//
// The environment can set it instead: TERN_TLS_DOMAINS, a comma-separated
// list, with TERN_TLS_EMAIL and TERN_TLS_CACHE.
func WithAutoTLS(a AutoTLS) Option {
	return func(o *options) { o.autoTLS = &a }
}

// autoTLSFromEnv reads TERN_TLS_DOMAINS and the rest.
func autoTLSFromEnv() *AutoTLS {
	domains := os.Getenv("TERN_TLS_DOMAINS")
	if domains == "" {
		return nil
	}
	a := &AutoTLS{Email: os.Getenv("TERN_TLS_EMAIL"), Cache: os.Getenv("TERN_TLS_CACHE"), HTTPAddr: os.Getenv("TERN_HTTP_ADDR")}
	for _, d := range strings.Split(domains, ",") {
		if d = strings.TrimSpace(d); d != "" {
			a.Domains = append(a.Domains, d)
		}
	}
	return a
}

// tlsSetup is how the server terminates TLS.
type tlsSetup struct {
	cfg *tls.Config
	// redirect serves plain HTTP, when set: ACME challenges and redirects.
	redirect http.Handler
	httpAddr string
}

func (a *App) tlsSetup() (*tlsSetup, error) {
	auto := a.opts.autoTLS
	if auto == nil && a.opts.tls == nil {
		auto = autoTLSFromEnv()
	}
	switch {
	case a.opts.tls != nil:
		cfg := a.opts.tls.Clone()
		if len(cfg.NextProtos) == 0 {
			cfg.NextProtos = []string{"h2", "http/1.1"}
		}
		return &tlsSetup{cfg: a.withHelloHooks(cfg)}, nil
	case auto != nil:
		if len(auto.Domains) == 0 {
			return nil, errors.New("kit: AutoTLS needs at least one domain")
		}
		cache := auto.Cache
		if cache == "" {
			cache = "tern-certs"
		}
		m := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			Cache:      autocert.DirCache(cache),
			HostPolicy: autocert.HostWhitelist(auto.Domains...),
			Email:      auto.Email,
		}
		if auto.DirectoryURL != "" {
			m.Client = &acme.Client{DirectoryURL: auto.DirectoryURL}
		}
		cfg := m.TLSConfig() // h2, http/1.1 and acme-tls/1
		cfg.MinVersion = tls.VersionTLS12
		s := &tlsSetup{cfg: a.withHelloHooks(cfg), httpAddr: auto.HTTPAddr}
		if s.httpAddr == "" {
			s.httpAddr = ":80"
		}
		if s.httpAddr != "-" {
			s.redirect = m.HTTPHandler(http.HandlerFunc(redirectHTTPS))
		}
		return s, nil
	}
	return nil, nil
}

// redirectHTTPS sends plain HTTP requests to the same URL over HTTPS.
func redirectHTTPS(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	target := "https://" + host + r.URL.RequestURI()
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
		return
	}
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// withHelloHooks makes cfg call the plugins' OnClientHello functions.
func (a *App) withHelloHooks(cfg *tls.Config) *tls.Config {
	if len(a.opts.helloHooks) == 0 {
		return cfg
	}
	hooks := a.opts.helloHooks
	next := cfg.GetConfigForClient
	cfg.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		for _, h := range hooks {
			h(hello)
		}
		if next != nil {
			return next(hello)
		}
		return nil, nil
	}
	return cfg
}

// The App terminates TLS itself rather than through http.Server, so that
// plugins can see a connection's decrypted bytes (see [Setup.WrapConn]).
// http.Server then gets plain connections and speaks HTTP/2 on them as it
// would over TLS (Protocols.SetUnencryptedHTTP2), and the App gives each
// request its TLS state back.

// listener accepts the server's connections, terminating TLS if t is set,
// and wraps them as the plugins asked.
type listener struct {
	net.Listener
	tls   *tls.Config
	wraps []func(net.Conn) net.Conn
}

func (l *listener) Accept() (net.Conn, error) {
	c, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if l.tls == nil && len(l.wraps) == 0 {
		// As it is, so http.Server keeps the TCP connection's sendfile
		// and half-close.
		return c, nil
	}
	var tc *tls.Conn
	if l.tls != nil {
		tc = tls.Server(c, l.tls)
		c = tc
	}
	for _, w := range l.wraps {
		c = w(c)
	}
	return &serverConn{Conn: c, tls: tc}, nil
}

// serverConn is a connection as the server has it.
type serverConn struct {
	net.Conn
	tls *tls.Conn // nil for plain connections
}

// CloseWrite half-closes the connection, as http.Server does before
// closing one, when what it wraps can.
func (c *serverConn) CloseWrite() error {
	if cw, ok := c.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return nil
}

// NetConn returns the connection as the plugins' wrappers left it.
func (c *serverConn) NetConn() net.Conn { return c.Conn }

type connKey struct{}

// WrapListener wraps ln as Serve does: terminating TLS if the App has it
// (WithTLS, WithAutoTLS) and applying the plugins' connection wrappers. Use
// it with ConfigureServer to serve the App with an http.Server of your
// own.
func (a *App) WrapListener(ln net.Listener) (net.Listener, error) {
	t, err := a.tlsSetup()
	if err != nil {
		return nil, err
	}
	a.tls = t
	l := &listener{Listener: ln, wraps: a.opts.connWraps}
	if t != nil {
		l.tls = t.cfg
	}
	return l, nil
}

// ConfigureServer sets srv up for listeners from WrapListener: HTTP/2 on
// the connections it terminated TLS on, and the plugins' connection
// contexts.
func (a *App) ConfigureServer(srv *http.Server) {
	if a.tls != nil {
		var p http.Protocols
		p.SetHTTP1(true)
		p.SetUnencryptedHTTP2(true)
		srv.Protocols = &p
	}
	hooks := a.opts.connContexts
	prev := srv.ConnContext
	srv.ConnContext = func(ctx context.Context, c net.Conn) context.Context {
		if prev != nil {
			ctx = prev(ctx, c)
		}
		ctx = context.WithValue(ctx, connKey{}, c)
		plugged := c
		if sc, ok := c.(*serverConn); ok {
			plugged = sc.Conn
		}
		for _, h := range hooks {
			ctx = h(ctx, plugged)
		}
		return ctx
	}
}

// restoreTLS gives r the TLS state of a connection the App terminated TLS
// on, which http.Server can't know.
func restoreTLS(r *http.Request) {
	if r.TLS != nil {
		return
	}
	if c, ok := r.Context().Value(connKey{}).(*serverConn); ok && c.tls != nil {
		st := c.tls.ConnectionState()
		r.TLS = &st
	}
}

// serveRedirects serves plain HTTP for ACME and redirects until ctx ends.
func serveRedirects(ctx context.Context, t *tlsSetup, errc chan<- error) {
	srv := &http.Server{Addr: t.httpAddr, Handler: t.redirect, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	// SO_REUSEPORT as for HTTPS, so a new release can take over port 80
	// too.
	ln, err := listenTCP(t.httpAddr)
	if err != nil {
		errc <- err
		return
	}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errc <- err
	}
}

// ReadFrom implements io.ReaderFrom, for sendfile on plain connections.
func (c *serverConn) ReadFrom(r io.Reader) (int64, error) {
	if rf, ok := c.Conn.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	return io.Copy(struct{ io.Writer }{c.Conn}, r)
}
