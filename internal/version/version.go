// Package version reports the version of the Tern build.
package version

import "runtime/debug"

// Version is set at build time with -ldflags "-X .../internal/version.Version=v0.1.0".
// When unset, it falls back to the module version recorded by the Go toolchain.
var Version = ""

// String returns the best available version string for this build.
func String() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
