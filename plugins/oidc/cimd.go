package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Client ID Metadata Documents (draft-ietf-oauth-client-id-metadata-
// document, and the MCP authorization spec): a client whose ID is an HTTPS
// URL publishes its metadata at that URL, and the provider fetches it on
// first use, so clients such as AI assistants needn't be registered ahead
// of time. The provider fetches URLs that strangers name, so the fetch is
// guarded against server-side request forgery: HTTPS only, public
// addresses only, no redirects, a size limit and a timeout.

const (
	metadataMaxBytes = 64 << 10
	metadataTimeout  = 5 * time.Second
	metadataMinTTL   = 5 * time.Minute
	metadataMaxTTL   = 24 * time.Hour
	metadataCacheMax = 1000
)

type cachedClient struct {
	client  *Client
	err     error
	expires time.Time
}

// metadataClients caches clients from metadata documents, and failures
// for a short while, so a bad URL isn't fetched on every request.
type metadataClients struct {
	mu      sync.Mutex
	entries map[string]cachedClient
}

// isMetadataURL reports whether id is a client ID that names a metadata
// document: an https URL with a path, and nothing else.
func isMetadataURL(id string) bool {
	u, err := url.Parse(id)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.Path != "" && u.Path != "/" &&
		u.User == nil && u.RawQuery == "" && u.Fragment == "" && !strings.Contains(u.Path, "/./") && !strings.Contains(u.Path, "/../")
}

// lookupClient returns a registered client, or with
// ClientMetadataDocuments, the client described at an https client ID.
func (p *Plugin) lookupClient(ctx context.Context, id string) (*Client, error) {
	if c := p.clients[id]; c != nil {
		return c, nil
	}
	if !p.opts.ClientMetadataDocuments || !isMetadataURL(id) {
		return nil, errUnknownClient
	}
	p.metadata.mu.Lock()
	if e, ok := p.metadata.entries[id]; ok && time.Now().Before(e.expires) {
		p.metadata.mu.Unlock()
		return e.client, e.err
	}
	p.metadata.mu.Unlock()

	c, ttl, err := p.fetchClient(ctx, id)
	if err != nil {
		p.log.Info("oidc: client metadata document rejected", "client_id", id, "err", err)
		ttl = time.Minute
	}
	p.metadata.mu.Lock()
	defer p.metadata.mu.Unlock()
	if p.metadata.entries == nil {
		p.metadata.entries = map[string]cachedClient{}
	}
	if len(p.metadata.entries) >= metadataCacheMax {
		now := time.Now()
		for k, e := range p.metadata.entries {
			if now.After(e.expires) {
				delete(p.metadata.entries, k)
			}
		}
		if len(p.metadata.entries) >= metadataCacheMax {
			clear(p.metadata.entries)
		}
	}
	p.metadata.entries[id] = cachedClient{client: c, err: err, expires: time.Now().Add(ttl)}
	return c, err
}

var errUnknownClient = errors.New("unknown client")

// clientMetadata is the part of a metadata document the provider uses
// (RFC 7591 §2 names).
type clientMetadata struct {
	ClientID                string   `json:"client_id"`
	ClientName              string   `json:"client_name"`
	ClientURI               string   `json:"client_uri"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope"`
}

func (p *Plugin) fetchClient(ctx context.Context, id string) (*Client, time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, metadataTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, id, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := p.metadataHTTP().Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("fetching: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("fetching: %s", res.Status)
	}
	if mt, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type")); mt != "application/json" && !strings.HasSuffix(mt, "+json") {
		return nil, 0, fmt.Errorf("the document is %q, not JSON", mt)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, metadataMaxBytes+1))
	if err != nil {
		return nil, 0, fmt.Errorf("reading: %w", err)
	}
	if len(body) > metadataMaxBytes {
		return nil, 0, errors.New("the document is too large")
	}
	var m clientMetadata
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, 0, fmt.Errorf("the document isn't valid JSON: %w", err)
	}
	c, err := clientFromMetadata(id, m)
	if err != nil {
		return nil, 0, err
	}
	return c, cacheTTL(res.Header.Get("Cache-Control")), nil
}

// clientFromMetadata checks a metadata document and makes the client it
// describes. Such clients are always public and never trusted: users
// approve each on the consent screen.
func clientFromMetadata(id string, m clientMetadata) (*Client, error) {
	if m.ClientID != id {
		return nil, errors.New("the document's client_id doesn't match its URL")
	}
	if strings.TrimSpace(m.ClientName) == "" {
		return nil, errors.New("the document has no client_name")
	}
	if len(m.RedirectURIs) == 0 {
		return nil, errors.New("the document has no redirect_uris")
	}
	for _, r := range m.RedirectURIs {
		if !redirectAllowed(r) {
			return nil, fmt.Errorf("redirect URI %q must be https, or http on localhost", r)
		}
	}
	if m.TokenEndpointAuthMethod != "" && m.TokenEndpointAuthMethod != "none" {
		return nil, fmt.Errorf("token_endpoint_auth_method %q isn't supported; use none", m.TokenEndpointAuthMethod)
	}
	grants := []string{"authorization_code", "refresh_token"}
	for _, g := range m.GrantTypes {
		if !slices.Contains(grants, g) {
			return nil, fmt.Errorf("grant type %q isn't supported for these clients", g)
		}
	}
	for _, rt := range m.ResponseTypes {
		if rt != "code" {
			return nil, fmt.Errorf("response type %q isn't supported", rt)
		}
	}
	name := strings.TrimSpace(m.ClientName)
	if len(name) > 100 {
		name = name[:100]
	}
	return &Client{ID: id, Public: true, Name: name, RedirectURIs: m.RedirectURIs, fromMetadata: true}, nil
}

// redirectAllowed accepts https redirect URIs, and http ones on loopback
// addresses for apps on the user's machine. Other schemes would let a
// stranger's document send codes to an app it doesn't own.
func redirectAllowed(r string) bool {
	u, err := url.Parse(r)
	if err != nil || u.Fragment != "" || u.Host == "" || u.User != nil {
		return false
	}
	return u.Scheme == "https" || (u.Scheme == "http" && isLoopback(u.Hostname()))
}

func isLoopback(h string) bool {
	if h == "localhost" {
		return true
	}
	a, err := netip.ParseAddr(h)
	return err == nil && a.IsLoopback()
}

// cacheTTL is how long to keep a document: its max-age, within bounds.
func cacheTTL(cc string) time.Duration {
	ttl := time.Hour
	for _, d := range strings.Split(cc, ",") {
		d = strings.TrimSpace(strings.ToLower(d))
		if v, ok := strings.CutPrefix(d, "max-age="); ok {
			if n, err := strconv.Atoi(v); err == nil {
				ttl = time.Duration(n) * time.Second
			}
		}
		if d == "no-store" || d == "no-cache" {
			ttl = 0
		}
	}
	return min(max(ttl, metadataMinTTL), metadataMaxTTL)
}

// metadataHTTP is the client that fetches metadata documents: a dialer
// that refuses anything but public addresses, checked on the address
// actually dialed, so DNS can't point it at the app's own network.
func (p *Plugin) metadataHTTP() *http.Client {
	if p.opts.MetadataClient != nil {
		return p.opts.MetadataClient
	}
	p.metadataOnce.Do(func() {
		dialer := &net.Dialer{Timeout: metadataTimeout, Control: publicOnly}
		p.metadataClient = &http.Client{
			Timeout: metadataTimeout,
			Transport: &http.Transport{
				DialContext:            dialer.DialContext,
				TLSHandshakeTimeout:    metadataTimeout,
				ResponseHeaderTimeout:  metadataTimeout,
				MaxResponseHeaderBytes: 16 << 10,
			},
			CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirects aren't followed") },
		}
	})
	return p.metadataClient
}

// publicOnly refuses connections to loopback, private, link-local,
// multicast and unspecified addresses, and to ports other than 443.
func publicOnly(_, address string, _ syscall.RawConn) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if port != "443" {
		return fmt.Errorf("port %s isn't allowed", port)
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return err
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || isSharedOrReserved(a) {
		return fmt.Errorf("address %s isn't public", a)
	}
	return nil
}

var reservedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"), // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),  // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"), // benchmarking
	netip.MustParsePrefix("240.0.0.0/4"),   // reserved
	netip.MustParsePrefix("64:ff9b::/96"),  // NAT64, which can reach IPv4 private ranges
	netip.MustParsePrefix("2001:db8::/32"), // documentation
	netip.MustParsePrefix("fec0::/10"),     // site-local
}

func isSharedOrReserved(a netip.Addr) bool {
	for _, p := range reservedPrefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
