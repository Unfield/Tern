package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Unfield/Tern/internal/codegen"
)

func TestHideServerBundle(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := filepath.FromSlash(codegen.ServerOut)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(dir, "server.js")
	if err := os.WriteFile(bundle, []byte("bundle"), 0o644); err != nil {
		t.Fatal(err)
	}
	restore, err := hideServerBundle()
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != ".keep" {
		t.Fatalf("while hidden, the directory holds %v, want only .keep", entries)
	}
	restore()
	if b, err := os.ReadFile(bundle); err != nil || string(b) != "bundle" {
		t.Fatalf("after restore: %q, %v", b, err)
	}
	if _, err := os.Stat(dir + ".hidden"); !os.IsNotExist(err) {
		t.Fatalf("the hidden copy is left behind: %v", err)
	}
}
