package main

import (
	"testing"

	"github.com/fsnotify/fsnotify"
)

func TestClassifyIgnoresChmod(t *testing.T) {
	for _, rel := range []string{"main.go", "go.mod", "src/routes/+page.svelte", "src/routes/new/+page.server.go", "queries/users.sql"} {
		if got := classify(rel, fsnotify.Chmod); got != changeNone {
			t.Errorf("chmod of %s: got %v, want no change", rel, got)
		}
	}
	if got := classify("main.go", fsnotify.Write|fsnotify.Chmod); got != changeBuild {
		t.Errorf("write+chmod of main.go: got %v, want a build", got)
	}
	if got := classify("main.go", fsnotify.Write); got != changeBuild {
		t.Errorf("write of main.go: got %v, want a build", got)
	}
	if got := classify("src/routes/about/+page.svelte", fsnotify.Create); got != changeGen {
		t.Errorf("new page: got %v, want gen", got)
	}
}
