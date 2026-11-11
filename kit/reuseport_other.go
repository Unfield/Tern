//go:build !(linux || darwin || freebsd || netbsd || openbsd || dragonfly)

package kit

import "errors"

func setReusePort(uintptr) error {
	return errors.New("kit: TERN_REUSEPORT isn't supported on this system")
}
