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

	"github.com/Unfield/Tern/internal/version"
)

//go:embed all:template
var templateFS embed.FS

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
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: tern new [-module path] <directory>")
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
		fmt.Fprintf(stderr, "tern new: %v\n", err)
		return 1
	}

	data := templateData{Name: name, Module: *module, ClientVersion: clientVersion()}
	if err := writeTemplate(dir, data); err != nil {
		fmt.Fprintf(stderr, "tern new: %v\n", err)
		return 1
	}

	if !*skipInstall {
		// Pin the CLI as a Go tool, so the project runs `go tool tern` at
		// the same version as the kit package it imports.
		ref := "github.com/Unfield/Tern/cmd/tern@" + moduleVersion()
		for _, args := range [][]string{{"get", "-tool", ref}, {"mod", "tidy"}} {
			cmd := exec.Command("go", args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Fprintf(stderr, "warning: go %s failed:\n%s\nRun it yourself once the problem is fixed.\n", strings.Join(args, " "), out)
				break
			}
		}
	}

	fmt.Fprintf(stdout, `Created %s.

Next steps:

  cd %s
  npm install
  npm run dev

`, dir, dir)
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

func writeTemplate(dir string, data templateData) error {
	gomod := fmt.Sprintf("module %s\n\ngo 1.26\n", data.Module)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		return err
	}
	return fs.WalkDir(templateFS, "template", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(p, "template/")
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

// clientVersion is the matching @unfield/tern npm version range.
func clientVersion() string {
	if v := version.String(); strings.HasPrefix(v, "v") {
		return "^" + strings.TrimPrefix(v, "v")
	}
	return "latest"
}
