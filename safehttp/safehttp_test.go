package safehttp

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestPublic(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8": true, "2606:4700:4700::1111": true,
		"127.0.0.1": false, "10.1.2.3": false, "172.16.0.1": false, "192.168.1.1": false,
		"169.254.169.254": false, // cloud metadata
		"100.64.0.1":      false, "0.0.0.0": false, "::1": false, "fe80::1": false, "fc00::1": false,
		"::ffff:127.0.0.1": false, "64:ff9b::a00:1": false, "2002:a00:1::": false, "224.0.0.1": false,
	} {
		if got := Public(netip.MustParseAddr(addr)); got != want {
			t.Errorf("Public(%s) = %v, want %v", addr, got, want)
		}
	}
}

func port(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	_, p, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// get fetches u and closes the body, for the error.
func get(c *http.Client, u string) error {
	res, err := c.Get(u)
	if err == nil {
		_ = res.Body.Close()
	}
	return err
}

func TestClient(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/big":
			_, _ = w.Write([]byte(strings.Repeat("x", 100)))
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		default:
			_, _ = w.Write([]byte("secret"))
		}
	}))
	defer internal.Close()
	p := port(t, internal)

	// The default refuses the loopback server, as it would the metadata
	// service or the database.
	err := get(New(Options{Ports: []string{p}}), internal.URL)
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("loopback: %v, want ErrBlocked", err)
	}
	// And names that resolve to it.
	if err := get(New(Options{Ports: []string{p}}), "http://localhost:"+p); !errors.Is(err, ErrBlocked) {
		t.Errorf("localhost: %v", err)
	}
	// Ports other than 80 and 443, by default.
	if err := get(New(Options{Allow: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}), internal.URL); !errors.Is(err, ErrBlocked) || !strings.Contains(err.Error(), "port") {
		t.Errorf("another port: %v", err)
	}

	allowed := New(Options{Ports: []string{p}, Allow: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, MaxBody: 10, MaxRedirects: 2})
	res, err := allowed.Get(internal.URL)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if string(b) != "secret" {
		t.Errorf("allowed: %q", b)
	}
	if err := get(allowed, internal.URL+"/big"); !errors.Is(err, ErrTooLarge) {
		t.Errorf("a body past MaxBody: %v", err)
	}
	if err := get(allowed, internal.URL+"/loop"); !errors.Is(err, ErrBlocked) || !strings.Contains(err.Error(), "redirects") {
		t.Errorf("redirects: %v", err)
	}
	if err := get(allowed, "file:///etc/passwd"); err == nil {
		t.Error("a file: URL")
	}
}

func TestCappedStreamedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush() // no Content-Length
		_, _ = w.Write([]byte(strings.Repeat("y", 50)))
	}))
	defer srv.Close()
	c := New(Options{Ports: []string{port(t, srv)}, Allow: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}, MaxBody: 20})
	res, err := c.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if _, err := io.ReadAll(res.Body); !errors.Is(err, ErrTooLarge) {
		t.Errorf("reading past MaxBody: %v", err)
	}
}
