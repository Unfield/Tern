package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"

	"github.com/Unfield/Tern/internal/ui"
	"github.com/Unfield/Tern/plugins"
)

// cmdAdd adds a plugin to the app: it goes and gets its module when it's
// not Tern's own, and adds it to kit.WithPlugins in main.go.
func cmdAdd(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dry := fs.Bool("n", false, "print the change to main.go, and change nothing")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: tern add [-n] <plugin>...\n\nAdds plugins to the app: a name from the list below, or any Go package with a plugin, such as github.com/acme/tern-thing.")
		listPlugins(stderr)
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	p := ui.New(stdout)
	if fs.NArg() == 0 {
		listPlugins(stdout)
		return 0
	}
	root, _ := os.Getwd()
	for _, name := range fs.Args() {
		if err := addPlugin(root, name, *dry, stdout); err != nil {
			ui.New(stderr).Error(err)
			return 1
		}
	}
	if !*dry {
		p.Println()
	}
	return 0
}

func listPlugins(w io.Writer) {
	p := ui.New(w)
	p.Println()
	rows := [][]string{{"PLUGIN", "", "WHAT IT DOES"}}
	for _, pl := range plugins.All() {
		by := "Tern"
		if !pl.Official() {
			by = pl.Author
		}
		rows = append(rows, []string{pl.Name, by, pl.Description})
	}
	p.Table(rows, func(col int, cell string) string {
		switch col {
		case 0:
			return p.Bold(cell)
		case 1:
			return p.Dim(cell)
		}
		return cell
	})
	p.Printf("\n  %s\n\n", p.Dim("More, and how to list yours: https://tern.land/plugins"))
}

// addPlugin adds one plugin to the app in root.
func addPlugin(root, name string, dry bool, stdout io.Writer) error {
	p := ui.New(stdout)
	pl, known := plugins.Find(name)
	if !known {
		if !strings.Contains(name, "/") {
			return fmt.Errorf("there's no plugin called %q: run tern add to see the list, or give a Go package, such as github.com/acme/tern-thing", name)
		}
		pl = plugins.Plugin{Import: name, Module: name}
	}
	if pl.Add == "" && known {
		p.Printf("\n  %s %s needs configuration before it's added: %s\n  %s\n", p.Yellow("!"), p.Bold(pl.Name), pl.Notes, p.Cyan(pl.Docs))
		return nil
	}
	if pl.Module != "" && !dry {
		p.Printf("  %s go get %s\n", p.Dim("→"), pl.Module)
		cmd := exec.Command("go", "get", pl.Module+"@latest")
		cmd.Dir = root
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go get %s: %w\n%s", pl.Module, err, strings.TrimSpace(out.String()))
		}
	}
	if pl.Add == "" {
		expr, err := findConstructor(root, pl.Import)
		if err != nil {
			return err
		}
		pl.Add = expr
	}
	file, err := mainFile(root)
	if err != nil {
		return err
	}
	src, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	out, changed, err := addToMain(src, pl)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(file), err)
	}
	rel, _ := filepath.Rel(root, file)
	if !changed {
		p.Printf("  %s %s is already in %s\n", p.Dim("·"), p.Bold(pluginName(pl)), rel)
		return nil
	}
	if dry {
		_, _ = stdout.Write(out)
		return nil
	}
	if err := os.WriteFile(file, out, 0o644); err != nil {
		return err
	}
	p.Printf("  %s added %s to %s: %s\n", p.Green("✓"), p.Bold(pluginName(pl)), rel, p.Dim(pl.Add))
	if pl.Notes != "" {
		p.Printf("    %s\n", pl.Notes)
	}
	if pl.Docs != "" {
		p.Printf("    %s\n", p.Cyan(pl.Docs))
	}
	return nil
}

func pluginName(pl plugins.Plugin) string {
	if pl.Name != "" {
		return pl.Name
	}
	return pl.Import
}

// mainFile is the Go file of the app's main package that calls ternApp.
func mainFile(root string) (string, error) {
	matches, _ := filepath.Glob(filepath.Join(root, "*.go"))
	for _, f := range matches {
		if strings.HasSuffix(f, "_test.go") || strings.HasSuffix(f, ".gen.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err == nil && bytes.Contains(b, []byte("ternApp(")) {
			return f, nil
		}
	}
	return "", errors.New("no Go file in this directory calls ternApp(...): run tern add in the app's directory")
}

// addToMain adds pl to the plugins of the ternApp call in src: to its
// kit.WithPlugins, or in one of its own. It reports whether it changed
// anything; a plugin already there is left alone.
func addToMain(src []byte, pl plugins.Plugin) ([]byte, bool, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "main.go", src, parser.ParseComments)
	if err != nil {
		return nil, false, err
	}
	alias := path.Base(pl.Import)
	for _, imp := range f.Imports {
		if strings.Trim(imp.Path.Value, `"`) == pl.Import && imp.Name != nil {
			alias = imp.Name.Name
		}
	}
	expr := pl.Add
	if alias != path.Base(pl.Import) {
		expr = strings.Replace(expr, path.Base(pl.Import)+".", alias+".", 1)
	}
	if _, err := parser.ParseExpr(expr); err != nil {
		return nil, false, fmt.Errorf("the plugin's expression %q isn't Go: %w", expr, err)
	}

	var app, with *ast.CallExpr
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, ok := call.Fun.(*ast.Ident); ok && id.Name == "ternApp" && app == nil {
			app = call
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "WithPlugins" && with == nil {
			with = call
		}
		return true
	})
	if app == nil {
		return nil, false, errors.New("no ternApp(...) call")
	}
	// Already there: the same package used in WithPlugins.
	if with != nil {
		for _, a := range with.Args {
			if usesPackage(a, alias) {
				return src, false, nil
			}
		}
	}
	off := func(p token.Pos) int { return fset.Position(p).Offset }
	insert := func(call *ast.CallExpr, text string) []byte {
		var at int
		var add string
		switch {
		case len(call.Args) == 0:
			at, add = off(call.Rparen), text
		case fset.Position(call.Rparen).Line > fset.Position(call.Args[len(call.Args)-1].End()).Line:
			// One per line: after the last one's comma.
			at = off(call.Args[len(call.Args)-1].End())
			if at < len(src) && src[at] == ',' {
				at++
			}
			add = "\n" + text + ","
		default:
			at, add = off(call.Rparen), ", "+text
		}
		return slices.Concat(src[:at], []byte(add), src[at:])
	}
	var out []byte
	if with != nil {
		out = insert(with, expr)
	} else {
		out = insert(app, "kit.WithPlugins("+expr+")")
	}

	f, err = parser.ParseFile(fset, "main.go", out, parser.ParseComments)
	if err != nil {
		return nil, false, err
	}
	imports := append([]string{pl.Import}, pl.Imports...)
	if with == nil {
		imports = append(imports, "github.com/Unfield/Tern/kit")
	}
	for _, imp := range imports {
		if alias != path.Base(pl.Import) && imp == pl.Import {
			continue // imported under its own name already
		}
		astutil.AddImport(fset, f, imp)
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), true, nil
}

func usesPackage(e ast.Expr, alias string) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == alias {
				found = true
			}
		}
		return !found
	})
	return found
}

// findConstructor looks in a package for a function that makes a plugin
// without arguments, or with only options, such as New(opts ...Option).
func findConstructor(root, importPath string) (string, error) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes, Dir: root}
	pkgs, err := packages.Load(cfg, importPath, "github.com/Unfield/Tern/kit")
	if err != nil {
		return "", err
	}
	var plugin *types.Interface
	var target *packages.Package
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			return "", fmt.Errorf("loading %s: %w", p.PkgPath, p.Errors[0])
		}
		if p.PkgPath == "github.com/Unfield/Tern/kit" {
			plugin, _ = p.Types.Scope().Lookup("Plugin").Type().Underlying().(*types.Interface)
		}
		if p.PkgPath == importPath {
			target = p
		}
	}
	if plugin == nil || target == nil {
		return "", fmt.Errorf("couldn't load %s", importPath)
	}
	var found []string
	scope := target.Types.Scope()
	for _, name := range scope.Names() {
		fn, ok := scope.Lookup(name).(*types.Func)
		if !ok || !fn.Exported() {
			continue
		}
		sig := fn.Type().(*types.Signature)
		if sig.Results().Len() != 1 || !types.Implements(sig.Results().At(0).Type(), plugin) {
			continue
		}
		if sig.Params().Len() == 0 || sig.Params().Len() == 1 && sig.Variadic() {
			found = append(found, name)
		}
	}
	switch {
	case slices.Contains(found, "New"):
		return target.Name + ".New()", nil
	case len(found) == 1:
		return target.Name + "." + found[0] + "()", nil
	case len(found) == 0:
		return "", fmt.Errorf("%s has no function that makes a plugin without arguments: add it to kit.WithPlugins in main.go yourself, as its docs say", importPath)
	default:
		return "", fmt.Errorf("%s has several plugins (%s): add the one you want to kit.WithPlugins in main.go", importPath, strings.Join(found, ", "))
	}
}
