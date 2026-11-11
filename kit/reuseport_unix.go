//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package kit

import (
	"runtime"
	"syscall"
)

// soReusePort is SO_REUSEPORT, which the syscall package doesn't name on
// every system.
func soReusePort() int {
	switch runtime.GOOS {
	case "linux":
		return 0xf
	default:
		return 0x200 // the BSDs and macOS
	}
}

func setReusePort(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, soReusePort(), 1)
}
