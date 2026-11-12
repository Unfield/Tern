// Package safehttp makes HTTP requests to addresses that come from users,
// such as webhooks, link previews and images, without letting them reach
// the app's own network (server-side request forgery):
//
//	client := safehttp.New(safehttp.Options{})
//	res, err := client.Get(userURL)
//
// The client only connects to public addresses, checked on the address it
// actually dials, so a name that resolves to 127.0.0.1 or 10.0.0.5, or a
// redirect to one, is refused. It ignores proxy settings, which would
// connect on its behalf, and limits time, redirects, ports and the size of
// response bodies.
package safehttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"syscall"
	"time"
)

// ErrBlocked is returned, wrapped, for a request to an address or port the
// client doesn't allow.
var ErrBlocked = errors.New("safehttp: blocked")

// ErrTooLarge is returned while reading a response body past MaxBody.
var ErrTooLarge = errors.New("safehttp: response body too large")

// Options configure a client. The zero value is a safe default.
type Options struct {
	// Timeout limits a whole request, redirects and body included; the
	// default is 10 seconds.
	Timeout time.Duration
	// MaxBody limits response bodies; the default is 10 MiB.
	MaxBody int64
	// MaxRedirects is how many redirects are followed; the default is 5,
	// and -1 follows none.
	MaxRedirects int
	// Ports are the ports it may connect to; the default is 80 and 443.
	Ports []string
	// Allow lists addresses it may connect to although they aren't public,
	// such as a service of your own on the private network.
	Allow []netip.Prefix
}

// New returns a client that only connects to public addresses.
func New(opts Options) *http.Client {
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}
	if opts.MaxBody <= 0 {
		opts.MaxBody = 10 << 20
	}
	if opts.MaxRedirects == 0 {
		opts.MaxRedirects = 5
	}
	if len(opts.Ports) == 0 {
		opts.Ports = []string{"80", "443"}
	}
	dialer := &net.Dialer{Timeout: opts.Timeout, Control: func(_, address string, _ syscall.RawConn) error {
		return checkAddress(address, opts)
	}}
	transport := &http.Transport{
		Proxy:                  nil, // a proxy would connect for us, past the check
		DialContext:            dialer.DialContext,
		TLSHandshakeTimeout:    opts.Timeout,
		ResponseHeaderTimeout:  opts.Timeout,
		MaxResponseHeaderBytes: 64 << 10,
		ForceAttemptHTTP2:      true,
	}
	return &http.Client{
		Timeout:   opts.Timeout,
		Transport: &limited{next: transport, max: opts.MaxBody},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if opts.MaxRedirects < 0 || len(via) > opts.MaxRedirects {
				return fmt.Errorf("%w: more than %d redirects", ErrBlocked, max(opts.MaxRedirects, 0))
			}
			return nil
		},
	}
}

// Public reports whether a is an address on the public internet: not
// loopback, private, link-local, multicast, unspecified, carrier-grade NAT
// or another reserved range.
func Public(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return false
	}
	return !slices.ContainsFunc(reserved, func(p netip.Prefix) bool { return p.Contains(a) })
}

var reserved = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),     // this network
	netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),  // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),  // documentation
	netip.MustParsePrefix("198.18.0.0/15"), // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),  // reserved
	netip.MustParsePrefix("64:ff9b::/96"), // NAT64, which can reach IPv4 private ranges
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001:db8::/32"), // documentation
	netip.MustParsePrefix("2002::/16"),     // 6to4, which embeds IPv4 addresses
	netip.MustParsePrefix("fec0::/10"),     // site-local
}

func checkAddress(address string, opts Options) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if !slices.Contains(opts.Ports, port) {
		return fmt.Errorf("%w: port %s", ErrBlocked, port)
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	if slices.ContainsFunc(opts.Allow, func(p netip.Prefix) bool { return p.Contains(a.Unmap()) }) {
		return nil
	}
	if !Public(a) {
		return fmt.Errorf("%w: %s isn't a public address", ErrBlocked, a.Unmap())
	}
	return nil
}

// limited caps response bodies.
type limited struct {
	next http.RoundTripper
	max  int64
}

func (l *limited) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return nil, fmt.Errorf("%w: scheme %q", ErrBlocked, req.URL.Scheme)
	}
	res, err := l.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	redirect := res.StatusCode >= 300 && res.StatusCode < 400
	if res.ContentLength > l.max && !redirect {
		_ = res.Body.Close()
		return nil, ErrTooLarge
	}
	res.Body = &capped{r: res.Body, left: l.max}
	return res, nil
}

type capped struct {
	r    io.ReadCloser
	left int64
}

func (c *capped) Read(p []byte) (int, error) {
	if c.left <= 0 {
		// Past the limit: one more byte means it's too large.
		var one [1]byte
		if n, _ := c.r.Read(one[:]); n > 0 {
			return 0, ErrTooLarge
		}
		return 0, io.EOF
	}
	if int64(len(p)) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	c.left -= int64(n)
	return n, err
}

func (c *capped) Close() error { return c.r.Close() }

// Get fetches url with a default client, bound to ctx.
func Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return defaultClient.Do(req)
}

var defaultClient = New(Options{})
