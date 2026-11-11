package main

import (
	"bufio"
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
	"saas":    "A full app: accounts with passkeys and two-step sign-in, a dashboard, settings and per-user data",
}

// bareTemplates are the small starters; the others are full apps, styled
// with Tern UI.
var bareTemplates = []string{"minimal", "blog", "api"}

// fullApp reports whether a template is a full app, which always uses
// Tern UI and brings its own layouts.
func fullApp(name string) bool { return name == "saas" }

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

type templateData struct {
	Name          string
	Module        string
	ClientVersion string
}

// stdin is where the interactive tern new reads answers; tests replace it.
var stdin io.Reader = os.Stdin

// stdinIsTerminal reports whether tern new may ask questions.
var stdinIsTerminal = func() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// cmdNew scaffolds a new project.
func cmdNew(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("new", flag.ContinueOnError)
	fl.SetOutput(stderr)
	module := fl.String("module", "", "Go module path (default: the directory name)")
	skipInstall := fl.Bool("skip-go-get", false, "do not run go get for the Tern module")
	tmpl := fl.String("template", "minimal", "starter template: minimal, blog, api, or saas for a full app")
	withUI := fl.Bool("ui", false, "style the starter with Tern UI")
	withGit := fl.Bool("git", false, "create a git repository with a first commit")
	npmInstall := fl.Bool("install", false, "run npm install")
	// Flags may come before or after the directory, as in
	// `npm create tern my-app -- -ui`.
	var positional []string
	for {
		if err := fl.Parse(args); err != nil {
			return 2
		}
		if fl.NArg() == 0 {
			break
		}
		positional = append(positional, fl.Arg(0))
		args = fl.Args()[1:]
	}
	var dir string
	interactive := false
	switch {
	case len(positional) == 1:
		dir = positional[0]
	case len(positional) == 0 && stdinIsTerminal():
		a, ok := askNew(stdout, stdin)
		if !ok {
			return 1
		}
		dir, *tmpl, *withUI, *withGit, *npmInstall = a.dir, a.template, a.ui, a.git, a.install
		interactive = true
	default:
		fmt.Fprintln(stderr, "usage: tern new [-template minimal|blog|api|saas] [-ui] [-git] [-install] [-module path] <directory>")
		return 2
	}
	if _, ok := templates[*tmpl]; !ok {
		fmt.Fprintf(stderr, "tern new: unknown template %q; choose minimal, blog, api or saas\n", *tmpl)
		return 2
	}
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
	if !interactive {
		p.Printf("\n  %s %s\n\n", p.Brand(p.Bold("tern new")), p.Dim(version.String()))
	}

	data := templateData{Name: name, Module: *module, ClientVersion: clientVersion()}
	s := p.Spin("Creating " + dir)
	if err := writeTemplate(dir, data, *tmpl, *withUI); err != nil {
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

	installed := false
	if *npmInstall {
		s := p.Spin("Installing npm packages")
		if out, err := runIn(dir, "npm", "install"); err != nil {
			s.Fail()
			p.Warn("npm install failed; run it yourself once the problem is fixed:\n%s", out)
		} else {
			s.Done("")
			installed = true
		}
	}
	if *withGit {
		s := p.Spin("Creating a git repository")
		var out []byte
		var err error
		for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"commit", "-q", "-m", "Create a Tern app"}} {
			if out, err = runIn(dir, "git", args...); err != nil {
				break
			}
		}
		if err != nil {
			s.Fail()
			p.Warn("git failed:\n%s", out)
		} else {
			s.Done("")
		}
	}

	p.Printf("\n  %s\n\n", p.Bold("Next steps"))
	steps := []string{"cd " + dir}
	if !installed {
		steps = append(steps, "npm install")
	}
	steps = append(steps, "npm run dev")
	for i, step := range steps {
		p.Printf("    %s %s\n", p.Dim(fmt.Sprintf("%d.", i+1)), p.Cyan(step))
	}
	p.Printf("\n")
	return 0
}

func runIn(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	return cmd.CombinedOutput()
}

type newAnswers struct {
	dir, template    string
	ui, git, install bool
}

// askNew asks for tern new's settings. It returns false if input ends
// before every question is answered.
func askNew(stdout io.Writer, in io.Reader) (newAnswers, bool) {
	p := ui.New(stdout)
	r := bufio.NewReader(in)
	a := newAnswers{}
	p.Printf("\n  %s %s\n\n", p.Brand(p.Bold("tern new")), p.Dim(version.String()))
	ask := func(question, def string) (string, bool) {
		p.Printf("  %s %s ", p.Bold(question), p.Dim("("+def+")"))
		line, err := r.ReadString('\n')
		if err != nil && line == "" {
			p.Printf("\n")
			return "", false
		}
		if line = strings.TrimSpace(line); line == "" {
			return def, true
		}
		return line, true
	}
	yes := func(question string, def bool) (bool, bool) {
		d := "Y/n"
		if !def {
			d = "y/N"
		}
		for {
			ans, ok := ask(question, d)
			if !ok {
				return false, false
			}
			if ans == d {
				return def, true // nothing typed
			}
			switch strings.ToLower(ans) {
			case "y", "yes":
				return true, true
			case "n", "no":
				return false, true
			}
		}
	}
	var ok bool
	for {
		if a.dir, ok = ask("Where should the project go?", "my-app"); !ok {
			return a, false
		}
		name := strings.ToLower(filepath.Base(filepath.Clean(a.dir)))
		if !nameRe.MatchString(name) {
			p.Printf("  %s\n", p.Dim("Use lowercase letters, digits, '.', '-' and '_'."))
			continue
		}
		if err := checkEmpty(a.dir); err != nil {
			p.Printf("  %s\n", p.Dim(err.Error()+"; pick another directory."))
			continue
		}
		break
	}
	p.Printf("\n")
	p.Printf("    %s %-6s %s\n", p.Dim("1."), "bare", p.Dim("A small starting point to build on"))
	p.Printf("    %s %-6s %s\n", p.Dim("2."), "full", p.Dim(templates["saas"]))
	full := false
	for {
		ans, ok := ask("Start with a bare app or a full one?", "1")
		if !ok {
			return a, false
		}
		switch strings.ToLower(ans) {
		case "1", "bare":
		case "2", "full", "saas":
			full = true
		default:
			continue
		}
		break
	}
	p.Printf("\n")
	if full {
		a.template, a.ui = "saas", true
	} else {
		for i, n := range bareTemplates {
			p.Printf("    %s %-8s %s\n", p.Dim(fmt.Sprintf("%d.", i+1)), n, p.Dim(templates[n]))
		}
		for a.template == "" {
			ans, ok := ask("Which template?", "1")
			if !ok {
				return a, false
			}
			for i, n := range bareTemplates {
				if ans == n || ans == fmt.Sprint(i+1) {
					a.template = n
				}
			}
		}
		p.Printf("\n")
		if a.ui, ok = yes("Style it with Tern UI?", true); !ok {
			return a, false
		}
	}
	if a.git, ok = yes("Create a git repository?", true); !ok {
		return a, false
	}
	if a.install, ok = yes("Run npm install?", true); !ok {
		return a, false
	}
	p.Printf("\n")
	return a, true
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

func writeTemplate(dir string, data templateData, name string, withUI bool) error {
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
	if name != "minimal" {
		if err := writeTree(dir, data, overlay, map[string]bool{"_remove": true}); err != nil {
			return err
		}
	}
	if !withUI || fullApp(name) {
		return nil // a full app has its own layouts, styled with Tern UI
	}
	if err := writeTree(dir, data, "templates/ui", nil); err != nil {
		return err
	}
	if name == "minimal" {
		return writeTree(dir, data, "templates/ui-minimal", nil)
	}
	return nil
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
