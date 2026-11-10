package main

import (
	"bytes"
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/Unfield/Tern/internal/ui"
	"github.com/Unfield/Tern/internal/version"
)

//go:embed all:template all:templates
var templateFS embed.FS

// templates are the starter templates; each but minimal adds to and
// replaces files of the base template in template/.
var templates = map[string]string{
	"minimal": "A page with a Go load and a form action",
	"blog":    "Markdown posts from src/content, prerendered, with an RSS feed",
	"api":     "Typed JSON endpoints with OpenAPI and a typed client",
}

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

type templateData struct {
	Name          string
	Module        string
	ClientVersion string
}

// cmdNew scaffolds a new project.
func cmdNew(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("new", flag.ContinueOnError)
	fl.SetOutput(stderr)
	module := fl.String("module", "", "Go module path (default: the directory name)")
	skipInstall := fl.Bool("skip-go-get", false, "do not run go get for the Tern module")
	tmpl := fl.String("template", "minimal", "starter template: minimal, blog or api")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: tern new [-module path] [-template minimal|blog|api] <directory>")
		return 2
	}
	if _, ok := templates[*tmpl]; !ok {
		fmt.Fprintf(stderr, "tern new: unknown template %q; choose minimal, blog or api\n", *tmpl)
		return 2
	}
	dir := fl.Arg(0)
	name := strings.ToLower(filepath.Base(filepath.Clean(dir)))
	if !nameRe.MatchString(name) {
		fmt.Fprintf(stderr, "tern new: %q is not a valid project name; use lowercase letters, digits, '.', '-' and '_'\n", name)
		return 2
	}
	if *module == "" {
		*module = name
	}
	if err := checkEmpty(dir); err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	p := ui.New(stdout)
	p.Printf("\n  %s %s\n\n", p.Brand(p.Bold("tern new")), p.Dim(version.String()))

	data := templateData{Name: name, Module: *module, ClientVersion: clientVersion()}
	s := p.Spin("Creating " + dir)
	if err := writeTemplate(dir, data, *tmpl); err != nil {
		s.Fail()
		ui.New(stderr).Error(err)
		return 1
	}
	s.Done("")

	if !*skipInstall {
		// Pin the CLI as a Go tool, so the project runs `go tool tern` at
		// the same version as the kit package it imports.
		ref := "github.com/Unfield/Tern/cmd/tern@" + moduleVersion()
		s := p.Spin("Adding Tern to go.mod")
		for _, args := range [][]string{{"get", "-tool", ref}, {"mod", "tidy"}} {
			cmd := exec.Command("go", args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				s.Fail()
				p.Warn("go %s failed; run it yourself once the problem is fixed:\n%s", strings.Join(args, " "), out)
				s = nil
				break
			}
		}
		if s != nil {
			s.Done(moduleVersion())
		}
	}

	p.Printf("\n  %s\n\n", p.Bold("Next steps"))
	p.Printf("    %s %s\n", p.Dim("1."), p.Cyan("cd "+dir))
	p.Printf("    %s %s\n", p.Dim("2."), p.Cyan("npm install"))
	p.Printf("    %s %s\n\n", p.Dim("3."), p.Cyan("npm run dev"))
	return 0
}

func checkEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%s is not empty", dir)
	}
	return nil
}

func writeTemplate(dir string, data templateData, name string) error {
	gomod := fmt.Sprintf("module %s\n\ngo 1.26\n", data.Module)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		return err
	}
	skip := map[string]bool{}
	overlay := "templates/" + name
	if raw, err := templateFS.ReadFile(overlay + "/_remove"); err == nil {
		for _, l := range strings.Fields(string(raw)) {
			skip[l] = true
		}
	}
	if err := writeTree(dir, data, "template", skip); err != nil {
		return err
	}
	if name == "minimal" {
		return nil // the base template alone
	}
	return writeTree(dir, data, overlay, map[string]bool{"_remove": true})
}

// writeTree writes the files under root in templateFS, rendering .tmpl
// files, except the relative paths in skip.
func writeTree(dir string, data templateData, root string, skip map[string]bool) error {
	return fs.WalkDir(templateFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(p, root+"/")
		if skip[rel] {
			return nil
		}
		content, err := templateFS.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.HasSuffix(rel, ".tmpl") {
			rel = strings.TrimSuffix(rel, ".tmpl")
			t, err := template.New(rel).Parse(string(content))
			if err != nil {
				return err
			}
			var buf bytes.Buffer
			if err := t.Execute(&buf, data); err != nil {
				return err
			}
			content = buf.Bytes()
		}
		// Dotfiles are stored with a leading underscore so tools that
		// ignore dotfiles still ship them.
		if base := path.Base(rel); strings.HasPrefix(base, "_") {
			rel = path.Join(path.Dir(rel), "."+base[1:])
		}
		out := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		return os.WriteFile(out, content, 0o644)
	})
}

// moduleVersion is the Tern Go module version new projects depend on.
func moduleVersion() string {
	if v := version.String(); strings.HasPrefix(v, "v") {
		return v
	}
	return "latest"
}

// clientVersion is the matching @ternkit/tern npm version range.
func clientVersion() string {
	if v := version.String(); strings.HasPrefix(v, "v") {
		return "^" + strings.TrimPrefix(v, "v")
	}
	return "latest"
}
