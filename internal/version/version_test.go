package version

import (
	"runtime/debug"
	"testing"
)

func TestStringPrefersLinkerValue(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })

	Version = "v1.2.3"
	if got := String(); got != "v1.2.3" {
		t.Fatalf("String() = %q, want %q", got, "v1.2.3")
	}
}

func TestStringFallback(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })

	Version = ""
	if got := String(); got == "" {
		t.Fatal("String() returned an empty string")
	}
}

func TestFromBuildInfo(t *testing.T) {
	tests := []struct {
		name string
		info debug.BuildInfo
		want string
	}{
		{"main module", debug.BuildInfo{Main: debug.Module{Path: modulePath, Version: "v0.3.0"}}, "v0.3.0"},
		{"local build", debug.BuildInfo{Main: debug.Module{Path: modulePath, Version: "(devel)"}}, "dev"},
		{"go tool dependency", debug.BuildInfo{
			Main: debug.Module{Path: "example.com/app"},
			Deps: []*debug.Module{{Path: "golang.org/x/tools", Version: "v0.52.0"}, {Path: modulePath, Version: "v0.3.1"}},
		}, "v0.3.1"},
		{"replaced dependency", debug.BuildInfo{
			Main: debug.Module{Path: "example.com/app"},
			Deps: []*debug.Module{{Path: modulePath, Version: "v0.0.0", Replace: &debug.Module{Path: "../.."}}},
		}, "dev"},
	}
	for _, tt := range tests {
		if got := fromBuildInfo(&tt.info); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
