package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestReadDotEnv(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	if got, err := readDotEnv(p); err != nil || got != nil {
		t.Fatalf("missing file: %v %v", got, err)
	}
	_ = os.WriteFile(p, []byte("# settings\nA=1\nexport B = two words # note\nEMPTY=\nC=\"x\\ny\"\nD='$raw'\n\n"), 0o644)
	got, err := readDotEnv(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"A=1", "B=two words", "C=x\ny", "D=$raw"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
	_ = os.WriteFile(p, []byte("not a line\n"), 0o644)
	if _, err := readDotEnv(p); err == nil {
		t.Fatal("bad line accepted")
	}
}
