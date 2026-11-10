package kit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	cases := []struct {
		name    string
		proxies []string
		peer    string
		xff     []string
		want    string
		trusted bool
	}{
		{name: "no proxy configured", peer: "203.0.113.7:1234", xff: []string{"1.2.3.4"}, want: "203.0.113.7", trusted: true},
		{name: "trusted proxy", proxies: []string{"10.0.0.0/8"}, peer: "10.0.0.2:443", xff: []string{"198.51.100.9"}, want: "198.51.100.9", trusted: true},
		{name: "spoofed header from untrusted peer", proxies: []string{"10.0.0.0/8"}, peer: "203.0.113.7:1234", xff: []string{"1.2.3.4"}, want: "203.0.113.7"},
		{name: "client-supplied entries are skipped", proxies: []string{"10.0.0.0/8"}, peer: "10.0.0.2:443", xff: []string{"1.2.3.4, 198.51.100.9"}, want: "198.51.100.9", trusted: true},
		{name: "several proxies", proxies: []string{"10.0.0.0/8", "192.0.2.1"}, peer: "10.0.0.2:443", xff: []string{"6.6.6.6, 198.51.100.9", "192.0.2.1, 10.1.1.1"}, want: "198.51.100.9", trusted: true},
		{name: "only proxies in the chain", proxies: []string{"10.0.0.0/8"}, peer: "10.0.0.2:443", xff: []string{"10.0.0.9"}, want: "10.0.0.9", trusted: true},
		{name: "no header from proxy", proxies: []string{"127.0.0.1"}, peer: "127.0.0.1:5000", want: "127.0.0.1", trusted: true},
		{name: "garbled entry stops the walk", proxies: []string{"10.0.0.0/8"}, peer: "10.0.0.2:443", xff: []string{"1.2.3.4, nonsense, 10.0.0.5"}, want: "10.0.0.5", trusted: true},
		{name: "IPv6 and mapped IPv4", proxies: []string{"::1", "10.0.0.0/8"}, peer: "[::1]:443", xff: []string{"2001:db8::1, ::ffff:10.0.0.3"}, want: "2001:db8::1", trusted: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ps, err := parseProxies(tc.proxies)
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.peer
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			got := ps.resolve(r)
			if got.ip != tc.want || got.trusted != tc.trusted {
				t.Fatalf("got %+v, want ip %s trusted %v", got, tc.want, tc.trusted)
			}
		})
	}
}

func TestTrustedProxiesInApp(t *testing.T) {
	if _, err := New(testManifest(), WithTrustedProxies("not-an-ip")); err == nil {
		t.Fatal("an invalid proxy should fail New")
	}

	var seen string
	https := false
	a, err := New(testManifest(), WithTrustedProxies("10.0.0.0/8"), WithMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen, https = ClientIP(r), isHTTPS(r)
			next.ServeHTTP(w, r)
		})
	}))
	if err != nil {
		t.Fatal(err)
	}
	serve := func(peer string) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = peer
		r.Header.Set("X-Forwarded-For", "198.51.100.9")
		r.Header.Set("X-Forwarded-Proto", "https")
		a.ServeHTTP(httptest.NewRecorder(), r)
	}
	serve("10.0.0.2:443")
	if seen != "198.51.100.9" || !https {
		t.Fatalf("through the proxy: ip %s https %v", seen, https)
	}
	serve("203.0.113.7:1234")
	if seen != "203.0.113.7" || https {
		t.Fatalf("from an untrusted peer: ip %s https %v", seen, https)
	}
}
