package codegen

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/Unfield/Tern/internal/formrules"
	"github.com/Unfield/Tern/internal/routes"
)

const kitPath = "github.com/Unfield/Tern/kit"

// routePackage is a type-checked route package.
type routePackage struct {
	dir     string // relative to the routes root
	pkgPath string
	alias   string // import alias in tern.gen.go

	load       types.Type // Load's data type, or nil
	layoutLoad types.Type // LayoutLoad's data type, or nil
	actions    []types.Type
	failures   []types.Type
	forms      map[string][]formrules.Field // the rules of each kit.FormAction, by action name
	actionsAny bool                         // Actions is not a literal we can inspect
	methods    []string
	endpoints  map[string]endpointSig // typed endpoints by method
	noSSR      bool                   // const SSR = false
	prerender  bool                   // const Prerender = true
	entries    bool                   // func Entries([ctx *kit.Ctx]) ([]map[string]string, error)
	entriesCtx bool                   // Entries takes a *kit.Ctx
}

// endpointSig is a typed endpoint's input and output; in is nil for
// func(*kit.Ctx) (Out, error), and both are nil for plain handlers.
type endpointSig struct {
	in, out types.Type
}

// analyze type-checks the route packages and validates their exports.
func analyze(root, routesDir string, m *routes.Manifest) (map[string]*routePackage, error) {
	dirs := map[string]*routes.GoPackage{}
	for _, r := range m.Routes {
		if r.Go != nil {
			dirs[r.Go.Dir] = r.Go
		}
		for _, l := range r.Layouts {
			if l.Go != nil {
				dirs[l.Go.Dir] = l.Go
			}
		}
	}
	if len(dirs) == 0 {
		return map[string]*routePackage{}, nil
	}

	sorted := make([]string, 0, len(dirs))
	for d := range dirs {
		sorted = append(sorted, d)
	}
	sort.Strings(sorted)

	patterns := make([]string, len(sorted))
	byAbs := map[string]string{}
	for i, d := range sorted {
		abs := filepath.Join(root, filepath.FromSlash(routesDir), filepath.FromSlash(d))
		patterns[i] = "./" + filepath.ToSlash(filepath.Join(routesDir, d))
		byAbs[realPath(abs)] = d
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir: root,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("loading route packages: %w", err)
	}

	var errs []string
	out := map[string]*routePackage{}
	for _, p := range pkgs {
		errs = append(errs, packageErrors(root, p)...)
		if len(p.Errors) > 0 {
			continue
		}
		if len(p.GoFiles) == 0 {
			errs = append(errs, fmt.Sprintf("codegen: package %s (%s) has no Go files", p.PkgPath, p.ID))
			continue
		}
		dir, ok := byAbs[realPath(filepath.Dir(p.GoFiles[0]))]
		if !ok {
			errs = append(errs, fmt.Sprintf("codegen: unexpected package %s", p.PkgPath))
			continue
		}
		rp := &routePackage{dir: dir, pkgPath: p.PkgPath}
		errs = append(errs, rp.inspect(p, dirs[dir])...)
		out[dir] = rp
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("%s", strings.Join(dedupe(errs), "\n"))
	}

	if len(out) < len(sorted) {
		// go/packages can return nothing without an error when go list
		// fails, for example when go.mod needs updating. Run go list
		// directly to show the real problem.
		return nil, fmt.Errorf("could not load the route packages: %s", goListError(root, patterns))
	}
	for i, d := range sorted {
		out[d].alias = fmt.Sprintf("route%d", i)
	}
	return out, nil
}

func (rp *routePackage) inspect(p *packages.Package, scanned *routes.GoPackage) []string {
	var errs []string
	scope := p.Types.Scope()
	pos := func(obj types.Object) string { return p.Fset.Position(obj.Pos()).String() }

	checkLoad := func(name string) types.Type {
		obj, ok := scope.Lookup(name).(*types.Func)
		if !ok {
			return nil
		}
		sig := obj.Type().(*types.Signature)
		data, ok := loadSignature(sig)
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: %s must have the signature func(ctx *kit.Ctx) (T, error), where T is a struct or map", pos(obj), name))
			return nil
		}
		return data
	}
	if scanned.Load {
		rp.load = checkLoad("Load")
	}
	boolConst := func(name string) (value, found bool) {
		obj := scope.Lookup(name)
		if obj == nil {
			return false, false
		}
		c, ok := obj.(*types.Const)
		if b, isBasic := obj.Type().Underlying().(*types.Basic); !ok || !isBasic || b.Info()&types.IsBoolean == 0 {
			errs = append(errs, fmt.Sprintf("%s: %s must be a boolean constant", pos(obj), name))
			return false, false
		}
		return constant.BoolVal(c.Val()), true
	}
	if v, ok := boolConst("SSR"); ok {
		rp.noSSR = !v
	}
	if v, ok := boolConst("Prerender"); ok {
		rp.prerender = v
	}
	if obj := scope.Lookup("Entries"); obj != nil {
		fn, ok := obj.(*types.Func)
		if !ok || !entriesSignature(fn.Type().(*types.Signature)) {
			errs = append(errs, fmt.Sprintf("%s: Entries must have the signature func(ctx *kit.Ctx) ([]map[string]string, error); the ctx parameter is optional", pos(obj)))
		} else {
			rp.entries = true
			rp.entriesCtx = fn.Type().(*types.Signature).Params().Len() == 1
		}
	}
	if scanned.LayoutLoad {
		rp.layoutLoad = checkLoad("LayoutLoad")
	}

	for _, m := range scanned.Methods {
		obj, ok := scope.Lookup(m).(*types.Func)
		if !ok {
			continue
		}
		sig := obj.Type().(*types.Signature)
		if !handlerSignature(sig) {
			in, out, ok := typedSignature(sig)
			if !ok {
				errs = append(errs, fmt.Sprintf("%s: %s must be func(ctx *kit.Ctx) error, func(ctx *kit.Ctx) (Out, error) or func(ctx *kit.Ctx, in In) (Out, error) with a struct In", pos(obj), m))
				continue
			}
			if rp.endpoints == nil {
				rp.endpoints = map[string]endpointSig{}
			}
			rp.endpoints[m] = endpointSig{in: in, out: out}
		}
		rp.methods = append(rp.methods, m)
	}

	if scanned.Actions {
		obj, ok := scope.Lookup("Actions").(*types.Var)
		if !ok || !isKitNamed(obj.Type(), "Actions") {
			at := ""
			if obj != nil {
				at = pos(obj) + ": "
			}
			errs = append(errs, at+"Actions must be a kit.Actions")
		} else {
			var formIns map[string]formInput
			rp.actions, formIns, rp.actionsAny = actionResults(p, obj)
			rp.failures = failTypes(p)
			if len(formIns) > 0 {
				rp.forms = map[string][]formrules.Field{}
				for name, in := range formIns {
					fields, err := formrules.FromTypes(in.t)
					if err != nil {
						errs = append(errs, fmt.Sprintf("%s: kit.FormAction %q: %v", in.pos, name, err))
						continue
					}
					rp.forms[name] = fields
				}
				if invalid := kitInvalid(p); invalid != nil {
					rp.failures = append(rp.failures, invalid)
				}
			}
		}
	}
	return errs
}

// loadSignature checks func(*kit.Ctx) (T, error) and returns T.
func loadSignature(sig *types.Signature) (types.Type, bool) {
	if sig.TypeParams().Len() > 0 || sig.Params().Len() != 1 || sig.Results().Len() != 2 {
		return nil, false
	}
	if !isCtxPtr(sig.Params().At(0).Type()) || !isError(sig.Results().At(1).Type()) {
		return nil, false
	}
	data := sig.Results().At(0).Type()
	if !isObjectType(data) {
		return nil, false
	}
	return data, true
}

func entriesSignature(sig *types.Signature) bool {
	if sig.TypeParams().Len() > 0 || sig.Results().Len() != 2 || !isError(sig.Results().At(1).Type()) {
		return false
	}
	if n := sig.Params().Len(); n > 1 || (n == 1 && !isCtxPtr(sig.Params().At(0).Type())) {
		return false
	}
	sl, ok := types.Unalias(sig.Results().At(0).Type()).Underlying().(*types.Slice)
	if !ok {
		return false
	}
	m, ok := types.Unalias(sl.Elem()).Underlying().(*types.Map)
	return ok && types.Identical(m.Key(), types.Typ[types.String]) && types.Identical(m.Elem(), types.Typ[types.String])
}

func handlerSignature(sig *types.Signature) bool {
	return sig.TypeParams().Len() == 0 && sig.Params().Len() == 1 && sig.Results().Len() == 1 &&
		isCtxPtr(sig.Params().At(0).Type()) && isError(sig.Results().At(0).Type())
}

// typedSignature checks func(*kit.Ctx[, In]) (Out, error), with In a
// struct, and returns In (or nil) and Out.
func typedSignature(sig *types.Signature) (in, out types.Type, ok bool) {
	if sig.TypeParams().Len() > 0 || sig.Results().Len() != 2 || !isError(sig.Results().At(1).Type()) {
		return nil, nil, false
	}
	switch sig.Params().Len() {
	case 1:
	case 2:
		in = sig.Params().At(1).Type()
		if _, isStruct := types.Unalias(in).Underlying().(*types.Struct); !isStruct {
			return nil, nil, false
		}
	default:
		return nil, nil, false
	}
	if !isCtxPtr(sig.Params().At(0).Type()) {
		return nil, nil, false
	}
	return in, sig.Results().At(0).Type(), true
}

func isCtxPtr(t types.Type) bool {
	p, ok := types.Unalias(t).(*types.Pointer)
	return ok && isKitNamed(p.Elem(), "Ctx")
}

func isKitNamed(t types.Type, name string) bool {
	n, ok := types.Unalias(t).(*types.Named)
	return ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == kitPath && n.Obj().Name() == name
}

func isError(t types.Type) bool {
	return types.Identical(t, types.Universe.Lookup("error").Type())
}

// isObjectType reports whether t encodes to a JSON object (or null).
func isObjectType(t types.Type) bool {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = types.Unalias(p.Elem())
	}
	if hasMethod(t, "MarshalJSON") {
		return true // trust custom encoders
	}
	switch u := t.Underlying().(type) {
	case *types.Struct:
		return true
	case *types.Map:
		return isStringKey(u.Key())
	case *types.Interface:
		return true // checked at runtime
	}
	return false
}

// actionResults returns the result type of each action in an Actions
// composite literal. It reports anyAction if an action's type is unknown.
// formInput is the input type of a kit.FormAction, with where it is.
type formInput struct {
	t   types.Type
	pos string
}

func actionResults(p *packages.Package, obj *types.Var) (results []types.Type, forms map[string]formInput, anyAction bool) {
	lit := findVarValue(p, obj)
	cl, ok := lit.(*ast.CompositeLit)
	if !ok {
		return nil, nil, true
	}
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if in, out, ok := formActionTypes(p, kv.Value); ok {
			if out != nil {
				results = append(results, out)
			} else {
				anyAction = true
			}
			if lit, ok := kv.Key.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if name, err := strconv.Unquote(lit.Value); err == nil {
					if forms == nil {
						forms = map[string]formInput{}
					}
					forms[name] = formInput{t: in, pos: p.Fset.Position(kv.Pos()).String()}
				}
			}
			continue
		}
		t := actionValueResult(p, kv.Value)
		if t == nil {
			anyAction = true
			continue
		}
		results = append(results, t)
	}
	return results, forms, anyAction
}

// kitInvalid returns the kit.Invalid type, which a FormAction fails with,
// from the kit package p uses.
func kitInvalid(p *packages.Package) types.Type {
	for _, obj := range p.TypesInfo.Uses {
		if obj.Pkg() != nil && obj.Pkg().Path() == kitPath {
			if inv := obj.Pkg().Scope().Lookup("Invalid"); inv != nil {
				return inv.Type()
			}
		}
	}
	return nil
}

// formActionTypes returns In and Out for kit.FormAction(f) where f is
// func(*kit.Ctx, In) (Out, error); out is nil if it is unknown.
func formActionTypes(p *packages.Package, e ast.Expr) (in, out types.Type, ok bool) {
	call, isCall := e.(*ast.CallExpr)
	if !isCall || len(call.Args) != 1 || !isKitFunc(p, call.Fun, "FormAction") {
		return nil, nil, false
	}
	sig, isSig := types.Unalias(p.TypesInfo.TypeOf(call.Args[0])).(*types.Signature)
	if !isSig || sig.Params().Len() != 2 || sig.Results().Len() != 2 {
		return nil, nil, false
	}
	out = sig.Results().At(0).Type()
	if isEmptyInterface(out) {
		out = nil
	}
	return sig.Params().At(1).Type(), out, true
}

// actionValueResult returns T for kit.Action(f) where f returns (T, error),
// or nil if the result type is not known.
func actionValueResult(p *packages.Package, e ast.Expr) types.Type {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || !isKitFunc(p, call.Fun, "Action") {
		return nil
	}
	sig, ok := types.Unalias(p.TypesInfo.TypeOf(call.Args[0])).(*types.Signature)
	if !ok || sig.Results().Len() != 2 {
		return nil
	}
	t := sig.Results().At(0).Type()
	if isEmptyInterface(t) {
		return nil
	}
	return t
}

// failTypes returns the types of the data passed to kit.Fail in p.
func failTypes(p *packages.Package) []types.Type {
	var out []types.Type
	for _, f := range p.Syntax {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || !isKitFunc(p, call.Fun, "Fail") {
				return true
			}
			t := p.TypesInfo.TypeOf(call.Args[1])
			if t == nil || isEmptyInterface(t) {
				return true
			}
			if b, ok := t.(*types.Basic); ok && b.Kind() == types.UntypedNil {
				return true
			}
			for _, seen := range out {
				if types.Identical(seen, t) {
					return true
				}
			}
			out = append(out, t)
			return true
		})
	}
	return out
}

func isKitFunc(p *packages.Package, fun ast.Expr, name string) bool {
	if ix, ok := fun.(*ast.IndexExpr); ok { // kit.Action[T](f)
		fun = ix.X
	}
	var id *ast.Ident
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		id = f.Sel
	case *ast.Ident:
		id = f
	default:
		return false
	}
	obj, ok := p.TypesInfo.Uses[id].(*types.Func)
	return ok && obj.Pkg() != nil && obj.Pkg().Path() == kitPath && obj.Name() == name
}

func findVarValue(p *packages.Package, obj *types.Var) ast.Expr {
	for _, f := range p.Syntax {
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, name := range vs.Names {
					if p.TypesInfo.Defs[name] == obj && i < len(vs.Values) {
						return vs.Values[i]
					}
				}
			}
		}
	}
	return nil
}

func isEmptyInterface(t types.Type) bool {
	i, ok := types.Unalias(t).Underlying().(*types.Interface)
	return ok && i.Empty()
}

// packageErrors formats a package's errors with paths relative to root.
// go list repeats type errors as a list error; prefer the precise ones.
func packageErrors(root string, p *packages.Package) []string {
	precise := false
	for _, e := range p.Errors {
		if e.Kind != packages.ListError {
			precise = true
		}
	}
	var out []string
	for _, e := range p.Errors {
		if precise && e.Kind == packages.ListError {
			continue
		}
		msg := e.Error()
		if rel, err := filepath.Rel(root, e.Pos); err == nil && e.Pos != "" && !strings.HasPrefix(rel, "..") {
			msg = rel + ": " + e.Msg
		}
		out = append(out, msg)
	}
	return out
}

func goListError(root string, patterns []string) string {
	cmd := exec.Command("go", append([]string{"list", "-e", "-f", "{{if .Error}}{{.Error}}{{end}}"}, patterns...)...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	msg := strings.TrimSpace(string(out))
	if msg == "" && err != nil {
		msg = err.Error()
	}
	if msg == "" {
		msg = "go list returned no packages"
	}
	return msg
}

func realPath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}

func dedupe(s []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// routeFile turns a route-relative path into a project-relative one.
func routeFile(rel string) string {
	if path.Base(rel) == routes.FilePageMarkdown {
		return path.Join(MarkdownPagesDir, path.Dir(rel), "page.svelte")
	}
	return path.Join(RoutesDir, rel)
}
