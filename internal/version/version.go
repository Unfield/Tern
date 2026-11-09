// Package version reports the version of the Tern build.
package version

import "runtime/debug"

// Version is set at build time with -ldflags "-X .../internal/version.Version=v0.1.0".
// When unset, it falls back to the module version recorded by the Go toolchain.
var Version = ""

const modulePath = "github.com/Unfield/Tern"

// String returns the best available version string for this build.
func String() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		return fromBuildInfo(info)
	}
	return "dev"
}

// fromBuildInfo finds Tern's version whether it was built as the main
// module (go install, go run) or as a dependency (go tool tern).
func fromBuildInfo(info *debug.BuildInfo) string {
	if info.Main.Path == modulePath {
		return usable(info.Main.Version)
	}
	for _, dep := range info.Deps {
		if dep.Path == modulePath {
			if dep.Replace != nil {
				return "dev" // a local checkout
			}
			return usable(dep.Version)
		}
	}
	return "dev"
}

func usable(v string) string {
	if v == "" || v == "(devel)" {
		return "dev"
	}
	return v
}
