package native

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCompileIR compiles $TERN_NATIVE_IR and writes the Go to
// $TERN_NATIVE_OUT, for working on the compiler with a real app.
func TestCompileIR(t *testing.T) {
	in, out := os.Getenv("TERN_NATIVE_IR"), os.Getenv("TERN_NATIVE_OUT")
	if in == "" || out == "" {
		t.Skip("set TERN_NATIVE_IR and TERN_NATIVE_OUT")
	}
	ir, err := LoadIR(in)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Compile(ir, Options{})
	if res != nil {
		_ = os.MkdirAll(out, 0o755)
		_ = os.WriteFile(filepath.Join(out, "native.go"), res.Source, 0o644)
		t.Logf("%d modules, %d problems", res.Modules, len(res.Problems))
		for _, p := range res.Problems {
			t.Logf("  %s", p)
		}
		for _, m := range res.Mismatches {
			t.Logf("  intrinsic not used: %s", m)
		}
	}
	if err != nil {
		t.Fatal(err)
	}
}
