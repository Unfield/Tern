package kit

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// WithTrustedProxies names the reverse proxies in front of the app, as IP
// addresses or CIDR ranges such as "10.0.0.0/8" or "127.0.0.1". Requests
// from these peers may set X-Forwarded-For, which [Ctx.ClientIP] and
// [ClientIP] then read, and X-Forwarded-Proto.
//
// Once any proxy is configured, X-Forwarded-Proto from other peers is
// ignored. Without this option, ClientIP is the connection's own address and
// X-Forwarded-Proto is honored from anyone, as before.
func WithTrustedProxies(proxies ...string) Option {
	return func(o *options) { o.trustedProxies = append(o.trustedProxies, proxies...) }
}

type clientKey struct{}

type clientInfo struct {
	ip string
	// trusted reports whether forwarding headers may be believed: the
	// peer is a trusted proxy, or no proxies are configured at all.
	trusted bool
}

type proxySet []netip.Prefix

func parseProxies(list []string) (proxySet, error) {
	var out proxySet
	for _, s := range list {
		s = strings.TrimSpace(s)
		if strings.Contains(s, "/") {
			p, err := netip.ParsePrefix(s)
			if err != nil {
				return nil, fmt.Errorf("kit: trusted proxy %q: %w", s, err)
			}
			out = append(out, p.Masked())
			continue
		}
		a, err := netip.ParseAddr(s)
		if err != nil {
			return nil, fmt.Errorf("kit: trusted proxy %q: %w", s, err)
		}
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

func (ps proxySet) contains(a netip.Addr) bool {
	a = a.Unmap()
	for _, p := range ps {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

func remoteAddr(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// resolve finds the client's address: the peer's, or, if the peer is a
// trusted proxy, the right-most address in X-Forwarded-For that isn't one.
// Addresses to the left of that were written by the client and can be
// anything, so they are never used.
func (ps proxySet) resolve(r *http.Request) clientInfo {
	peer := remoteAddr(r)
	if len(ps) == 0 {
		return clientInfo{ip: peer, trusted: true}
	}
	addr, err := netip.ParseAddr(peer)
	if err != nil || !ps.contains(addr) {
		return clientInfo{ip: peer}
	}
	ip := addr.Unmap().String()
	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		hops = append(hops, strings.Split(v, ",")...)
	}
	for i := len(hops) - 1; i >= 0; i-- {
		hop, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break // a garbled entry: keep the last address we could trust
		}
		ip = hop.Unmap().String()
		if !ps.contains(hop) {
			break
		}
	}
	return clientInfo{ip: ip, trusted: true}
}

func (ps proxySet) handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := ps.resolve(r)
		r = r.WithContext(context.WithValue(r.Context(), clientKey{}, info))
		next.ServeHTTP(w, requestContext(w, r, info.trusted))
	})
}

func client(r *http.Request) clientInfo {
	if info, ok := r.Context().Value(clientKey{}).(clientInfo); ok {
		return info
	}
	return clientInfo{ip: remoteAddr(r), trusted: true}
}

// ClientIP returns the address of the client that made r, taking
// [WithTrustedProxies] into account. Middleware and plugins can use it;
// handlers with a [Ctx] can call [Ctx.ClientIP].
func ClientIP(r *http.Request) string { return client(r).ip }

// ClientIP returns the client's IP address. Behind a reverse proxy,
// configure it with [WithTrustedProxies]; otherwise it is the proxy's
// address.
func (c *Ctx) ClientIP() string { return ClientIP(c.Request) }

// isHTTPS reports whether the client connected over HTTPS, believing
// X-Forwarded-Proto only from trusted peers.
func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return client(r).trusted && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}
