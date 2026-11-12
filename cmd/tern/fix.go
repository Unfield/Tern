package main

import (
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Unfield/Tern/internal/codegen"
	"github.com/Unfield/Tern/internal/routes"
	"github.com/Unfield/Tern/internal/ui"
)

// accessFile is what tern fix access writes into a route that has server
// code and no Access rule.
const accessFile = `package %s

import "github.com/Unfield/Tern/kit"

// Access was added by tern fix access when Tern began closing routes by
// default (ADR 0027): it keeps the route open to everyone, as it was.
// Tighten it where it should be: kit.SignedIn, or kit.Permission("…").
var Access = kit.Public
`

func cmdFix(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("fix", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: tern fix access\n\nMarks every route that has server code (loads, actions or endpoints) and no Access rule kit.Public, which keeps it open to everyone as before Tern closed routes by default. Review the list it prints and tighten each one that should be: kit.SignedIn, or kit.Permission(\"…\").")
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 || fs.Arg(0) != "access" {
		fs.Usage()
		return 2
	}
	e := ui.New(stderr)
	dirs, err := routes.MissingAccess(os.DirFS("."), codegen.RoutesDir)
	if err != nil {
		e.Error(err)
		return 1
	}
	p := ui.New(stdout)
	if len(dirs) == 0 {
		p.Println("Every route with server code has an Access rule.")
		return 0
	}
	p.Printf("Marked %s kit.Public, open to everyone as before:\n\n", plural(len(dirs), "route"))
	for _, dir := range dirs {
		rel := path.Join(codegen.RoutesDir, dir)
		file, err := writeAccess(filepath.FromSlash(rel))
		if err != nil {
			e.Error(err)
			return 1
		}
		p.Printf("  %s\n", filepath.ToSlash(file))
	}
	p.Println("\nTighten the ones that should be: var Access = kit.SignedIn, or kit.Permission(\"…\").")
	return 0
}

// writeAccess adds a file declaring Access = kit.Public to the route
// package in dir, and returns its name.
func writeAccess(dir string) (string, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	name := ""
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.PackageClauseOnly)
		if err != nil {
			return "", err
		}
		name = parsed.Name.Name
		break
	}
	if name == "" {
		return "", fmt.Errorf("%s: no Go package", dir)
	}
	file := filepath.Join(dir, "access.go")
	if _, err := os.Stat(file); err == nil {
		file = filepath.Join(dir, "access_tern.go")
	}
	return file, os.WriteFile(file, fmt.Appendf(nil, accessFile, name), 0o644)
}
