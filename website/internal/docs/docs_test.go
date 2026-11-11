package docs

import (
	"os"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	Root = "../../.." // the tests run in website/internal/docs
	os.Exit(m.Run())
}

func TestEveryPageRenders(t *testing.T) {
	ps, err := list()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range ps {
		d, err := Get(p.Slug)
		if err != nil {
			t.Fatalf("%s: %v", p.Source, err)
		}
		if d.HTML == "" || d.Title == "" {
			t.Errorf("%s: empty", p.Source)
		}
		// Links to other Markdown files in the repository become pages.
		for _, bad := range []string{`href="../`, `.md"`, `.md#`} {
			if strings.Contains(d.HTML, bad) && !strings.Contains(d.HTML, "github.com/Unfield/Tern/blob") {
				t.Errorf("%s: a link left as %q", p.Source, bad)
			}
		}
	}
	if _, err := Get("nonesuch"); err != ErrNotFound {
		t.Error(err)
	}
}

func TestRewrite(t *testing.T) {
	if _, err := list(); err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{
		"guide.md#logging":   "/docs/guide#logging",
		"adr/0014-logging.md": "/docs/decisions/0014-logging",
		"../examples/todos":  RepoURL + "/tree/main/examples/todos",
		"../kit/app.go":      RepoURL + "/blob/main/kit/app.go",
		"https://x.example":  "https://x.example",
		"#here":              "#here",
	} {
		if got := rewrite("docs/logging.md", in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}
