package kit

import (
	"context"
	"net"
	"os"
	"syscall"
)

// Under systemd with Type=notify, the App says when it is ready (after its
// start functions ran and it listens) and when it stops, so a new release
// can be started next to the old one and the old one stopped only once the
// new one serves. tern deploy's units work this way.

// sdNotify sends state to systemd's notification socket, if there is one.
func sdNotify(state string) {
	addr := os.Getenv("NOTIFY_SOCKET")
	if addr == "" {
		return
	}
	if addr[0] == '@' {
		addr = "\x00" + addr[1:] // the abstract namespace
	}
	c, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: addr, Net: "unixgram"})
	if err != nil {
		return
	}
	defer func() { _ = c.Close() }()
	_, _ = c.Write([]byte(state))
}

// listenConfig is how the App listens: with SO_REUSEPORT when
// TERN_REUSEPORT=1, so two releases can share the port while one takes
// over from the other (Linux and the BSDs).
func listenConfig() *net.ListenConfig {
	if os.Getenv("TERN_REUSEPORT") != "1" {
		return &net.ListenConfig{}
	}
	return &net.ListenConfig{Control: func(_, _ string, c syscall.RawConn) error {
		var serr error
		if err := c.Control(func(fd uintptr) { serr = setReusePort(fd) }); err != nil {
			return err
		}
		return serr
	}}
}

func listenTCP(addr string) (net.Listener, error) {
	return listenConfig().Listen(context.Background(), "tcp", addr)
}
