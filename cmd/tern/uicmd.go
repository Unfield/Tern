package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Unfield/Tern/internal/ui"
)

// cmdUI copies Tern UI components into the project, to own and change.
func cmdUI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (args[0] != "add" && args[0] != "list") {
		fmt.Fprintln(stderr, "usage: tern ui list\n       tern ui add [-dir src/lib/ui] [-force] <Component>...")
		return 2
	}
	src, err := uiSource(".")
	if err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	public, err := uiComponents(src)
	if err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	if args[0] == "list" {
		for _, c := range public {
			fmt.Fprintln(stdout, c)
		}
		return 0
	}

	fl := flag.NewFlagSet("ui add", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("dir", "src/lib/ui", "where to put the components")
	force := fl.Bool("force", false, "replace files that are already there")
	if err := fl.Parse(args[1:]); err != nil {
		return 2
	}
	if fl.NArg() == 0 {
		fmt.Fprintln(stderr, "tern ui add: name the components to add, such as Button or Dialog; tern ui list shows them")
		return 2
	}
	known := map[string]bool{}
	for _, c := range public {
		known[strings.ToLower(c)] = true
	}
	var want []string
	for _, name := range fl.Args() {
		name = strings.TrimSuffix(name, ".svelte")
		match := ""
		for _, c := range public {
			if strings.EqualFold(c, name) {
				match = c
			}
		}
		if match == "" {
			ui.New(stderr).Error(fmt.Errorf("there is no Tern UI component %q; tern ui list shows them", name))
			return 1
		}
		want = append(want, match+".svelte")
	}
	files, err := uiFiles(src, want)
	if err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	p := ui.New(stdout)
	if err := os.MkdirAll(*dir, 0o755); err != nil {
		ui.New(stderr).Error(err)
		return 1
	}
	var written, kept []string
	for _, f := range files {
		dst := filepath.Join(*dir, f)
		if _, err := os.Stat(dst); err == nil && !*force {
			kept = append(kept, f)
			continue
		}
		raw, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			ui.New(stderr).Error(err)
			return 1
		}
		if err := os.WriteFile(dst, rewriteUIImports(raw), 0o644); err != nil {
			ui.New(stderr).Error(err)
			return 1
		}
		written = append(written, f)
	}
	for _, f := range written {
		p.Printf("  %s %s\n", p.Green("✓"), filepath.ToSlash(filepath.Join(*dir, f)))
	}
	for _, f := range kept {
		p.Printf("  %s %s %s\n", p.Dim("·"), filepath.ToSlash(filepath.Join(*dir, f)), p.Dim("(already there; -force replaces it)"))
	}
	if rel := filepath.ToSlash(filepath.Clean(*dir)); rel == "src/lib" || strings.HasPrefix(rel, "src/lib/") {
		alias := "$lib" + strings.TrimPrefix(rel, "src/lib")
		p.Printf("\n  Import them from %s, such as:\n    %s\n\n", p.Cyan(alias),
			p.Cyan(fmt.Sprintf("import %s from '%s/%s';", strings.TrimSuffix(want[0], ".svelte"), alias, want[0])))
	}
	return 0
}

// uiSource finds Tern UI in the project's installed @ternkit/tern.
func uiSource(root string) (string, error) {
	dir := filepath.Join(root, "node_modules", "@ternkit", "tern", "dist", "ui")
	if _, err := os.Stat(filepath.Join(dir, "index.js")); err != nil {
		return "", errors.New("@ternkit/tern isn't installed here; run npm install first")
	}
	return dir, nil
}

var exportRe = regexp.MustCompile(`export \{ default as (\w+) \} from '\./(\w+)\.svelte'`)

// uiComponents lists the components Tern UI exports.
func uiComponents(src string) ([]string, error) {
	raw, err := os.ReadFile(filepath.Join(src, "index.js"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, m := range exportRe.FindAllStringSubmatch(string(raw), -1) {
		out = append(out, m[2])
	}
	sort.Strings(out)
	return out, nil
}

var localImportRe = regexp.MustCompile(`from '\./([\w.]+)'`)

// uiFiles returns the files the given components need: themselves and the
// local components and helpers they import, with their type declarations.
func uiFiles(src string, want []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	var visit func(f string) error
	visit = func(f string) error {
		if seen[f] {
			return nil
		}
		seen[f] = true
		raw, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			return err
		}
		out = append(out, f)
		if strings.HasSuffix(f, ".js") {
			if d := strings.TrimSuffix(f, ".js") + ".d.ts"; fileExists(filepath.Join(src, d)) && !seen[d] {
				seen[d] = true
				out = append(out, d)
			}
		}
		for _, m := range localImportRe.FindAllStringSubmatch(string(raw), -1) {
			if err := visit(m[1]); err != nil {
				return err
			}
		}
		return nil
	}
	for _, w := range want {
		if err := visit(w); err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

var runtimeImportRe = regexp.MustCompile(`from '\.\./(runtime|image)/[\w./]+'`)

// rewriteUIImports points imports of Tern's runtime at the package, since
// the copies no longer sit next to it.
func rewriteUIImports(src []byte) []byte {
	return runtimeImportRe.ReplaceAll(src, []byte(`from '@ternkit/tern'`))
}
