//go:build linux || darwin

package kit

import (
	"net"
	"path/filepath"
	"testing"
)

func TestReusePort(t *testing.T) {
	t.Setenv("TERN_REUSEPORT", "1")
	a, err := listenTCP("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	b, err := listenTCP(a.Addr().String())
	if err != nil {
		t.Fatalf("a second listener on the port: %v", err)
	}
	_ = b.Close()
}

func TestSDNotify(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notify")
	c, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: path, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	t.Setenv("NOTIFY_SOCKET", path)
	sdNotify("READY=1")
	buf := make([]byte, 64)
	n, err := c.Read(buf)
	if err != nil || string(buf[:n]) != "READY=1" {
		t.Fatalf("%q %v", buf[:n], err)
	}
}
