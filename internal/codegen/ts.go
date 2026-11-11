package codegen

import (
	"fmt"
	"go/types"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// tsEmitter converts Go types to TypeScript, following encoding/json.
// Named struct types become interface declarations so recursive types work.
type tsEmitter struct {
	names    map[*types.TypeName]string
	taken    map[string]bool
	decls    []string
	visiting map[types.Type]bool
}

// reservedNames are declared by every $types.d.ts.
var reservedNames = []string{
	"Expand", "Merge", "OptionalUnion", "Snippet",
	"RouteParams", "LayoutServerData", "LayoutData", "LayoutProps", "LayoutParentData",
	"PageServerData", "PageData", "PageProps", "ActionData",
}

func newTSEmitter() *tsEmitter {
	e := &tsEmitter{names: map[*types.TypeName]string{}, taken: map[string]bool{}, visiting: map[types.Type]bool{}}
	for _, n := range reservedNames {
		e.taken[n] = true
	}
	return e
}

// dataType returns the TypeScript type for a load's data: like typeOf, but
// a nil pointer or map becomes an empty object rather than null.
func (e *tsEmitter) dataType(t types.Type) string {
	t = types.Unalias(t)
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	return strings.TrimSuffix(e.typeOf(t), " | null")
}

func (e *tsEmitter) typeOf(t types.Type) string {
	t = types.Unalias(t)

	if n, ok := t.(*types.Named); ok {
		if obj := n.Obj(); obj.Pkg() != nil && obj.Pkg().Path() == "time" && obj.Name() == "Time" {
			return "string"
		}
		if obj := n.Obj(); obj.Pkg() != nil && obj.Pkg().Path() == "encoding/json" && obj.Name() == "RawMessage" {
			return "unknown"
		}
		if obj := n.Obj(); obj.Pkg() != nil && obj.Pkg().Path() == kitPath && obj.Name() == "Deferred" && n.TypeArgs().Len() == 1 {
			return "Promise<" + e.typeOf(n.TypeArgs().At(0)) + ">"
		}
	}
	if _, isPtr := t.(*types.Pointer); !isPtr {
		if hasMethod(t, "MarshalJSON") {
			return "unknown"
		}
		if hasMethod(t, "MarshalText") {
			return "string"
		}
	}

	switch u := t.(type) {
	case *types.Named:
		if st, ok := u.Underlying().(*types.Struct); ok && u.TypeArgs().Len() == 0 {
			return e.declare(u, st)
		}
		if e.visiting[u] {
			return "unknown" // recursive generic or non-struct type
		}
		e.visiting[u] = true
		defer delete(e.visiting, u)
		return e.typeOf(u.Underlying())
	case *types.Basic:
		return basicTS(u)
	case *types.Pointer:
		return orNull(e.typeOf(u.Elem()))
	case *types.Slice:
		if isByte(u.Elem()) {
			return "string" // base64
		}
		return "Array<" + e.typeOf(u.Elem()) + ">" // kit sends a nil slice as []
	case *types.Array:
		return "Array<" + e.typeOf(u.Elem()) + ">"
	case *types.Map:
		if !isStringKey(u.Key()) {
			return "unknown"
		}
		return orNull("Record<string, " + e.typeOf(u.Elem()) + ">")
	case *types.Struct:
		return e.structLiteral(u)
	}
	return "unknown"
}

func orNull(s string) string {
	if strings.HasSuffix(s, " | null") || s == "unknown" {
		return s
	}
	return s + " | null"
}

func basicTS(b *types.Basic) string {
	switch {
	case b.Info()&types.IsBoolean != 0:
		return "boolean"
	case b.Info()&types.IsString != 0:
		return "string"
	case b.Info()&(types.IsInteger|types.IsFloat) != 0:
		return "number"
	}
	return "unknown"
}

func isByte(t types.Type) bool {
	b, ok := types.Unalias(t).Underlying().(*types.Basic)
	return ok && b.Kind() == types.Uint8
}

func isStringKey(t types.Type) bool {
	if hasMethod(t, "MarshalText") {
		return true
	}
	b, ok := types.Unalias(t).Underlying().(*types.Basic)
	return ok && b.Info()&(types.IsString|types.IsInteger) != 0
}

func hasMethod(t types.Type, name string) bool {
	if types.NewMethodSet(t).Lookup(nil, name) != nil {
		return true
	}
	if _, isPtr := t.(*types.Pointer); !isPtr {
		if _, isIface := t.Underlying().(*types.Interface); !isIface {
			return types.NewMethodSet(types.NewPointer(t)).Lookup(nil, name) != nil
		}
	}
	return false
}

// declare emits an interface for a named struct and returns its TS name.
func (e *tsEmitter) declare(n *types.Named, st *types.Struct) string {
	obj := n.Obj()
	if name, ok := e.names[obj]; ok {
		return name
	}
	name := obj.Name()
	if e.taken[name] && obj.Pkg() != nil {
		name = obj.Pkg().Name() + "_" + obj.Name()
	}
	for i := 2; e.taken[name]; i++ {
		name = fmt.Sprintf("%s%d", obj.Name(), i)
	}
	e.taken[name] = true
	e.names[obj] = name

	body := e.structLiteral(st)
	doc := ""
	if obj.Pkg() != nil {
		doc = fmt.Sprintf("/** Go type %s.%s */\n", obj.Pkg().Path(), obj.Name())
	}
	e.decls = append(e.decls, fmt.Sprintf("%sexport interface %s %s", doc, name, body))
	return name
}

type jsonField struct {
	name     string
	typ      types.Type
	optional bool
	asString bool
	depth    int
	tagged   bool
	index    int
}

// structLiteral renders the JSON object type of a struct.
func (e *tsEmitter) structLiteral(st *types.Struct) string {
	fields := resolveFields(collectFields(st, 0, map[*types.Struct]bool{}))
	if len(fields) == 0 {
		return "{}"
	}
	var b strings.Builder
	b.WriteString("{\n")
	for _, f := range fields {
		ts := e.typeOf(f.typ)
		if f.asString {
			ts = "string"
		}
		opt := ""
		if f.optional {
			opt = "?"
		}
		fmt.Fprintf(&b, "\t%s%s: %s;\n", tsKey(f.name), opt, ts)
	}
	b.WriteString("}")
	return b.String()
}

func collectFields(st *types.Struct, depth int, seen map[*types.Struct]bool) []jsonField {
	if seen[st] {
		return nil
	}
	seen[st] = true
	defer delete(seen, st)

	var out []jsonField
	for i := range st.NumFields() {
		f := st.Field(i)
		tag := reflect.StructTag(st.Tag(i)).Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		optional := hasOpt(opts, "omitempty") || hasOpt(opts, "omitzero")

		if f.Embedded() && name == "" {
			ft := types.Unalias(f.Type())
			if p, ok := ft.(*types.Pointer); ok {
				ft = types.Unalias(p.Elem())
			}
			if est, ok := ft.Underlying().(*types.Struct); ok {
				for _, ef := range collectFields(est, depth+1, seen) {
					ef.index = len(out)
					out = append(out, ef)
				}
				continue
			}
		}
		if !f.Exported() {
			continue
		}
		jf := jsonField{name: f.Name(), typ: f.Type(), optional: optional, depth: depth, index: len(out)}
		if name != "" {
			jf.name, jf.tagged = name, true
		}
		if hasOpt(opts, "string") {
			if b, ok := types.Unalias(f.Type()).Underlying().(*types.Basic); ok && b.Info()&(types.IsNumeric|types.IsBoolean) != 0 {
				jf.asString = true
			}
		}
		out = append(out, jf)
	}
	return out
}

// resolveFields applies encoding/json's rules for duplicate names: the
// shallowest field wins; at equal depth a single tagged field wins, and
// otherwise all are dropped.
func resolveFields(fields []jsonField) []jsonField {
	byName := map[string][]jsonField{}
	var order []string
	for _, f := range fields {
		if _, ok := byName[f.name]; !ok {
			order = append(order, f.name)
		}
		byName[f.name] = append(byName[f.name], f)
	}
	var out []jsonField
	for _, name := range order {
		cands := byName[name]
		sort.SliceStable(cands, func(i, j int) bool { return cands[i].depth < cands[j].depth })
		top := cands[0].depth
		var best []jsonField
		for _, c := range cands {
			if c.depth == top {
				best = append(best, c)
			}
		}
		if len(best) == 1 {
			out = append(out, best[0])
			continue
		}
		var tagged []jsonField
		for _, c := range best {
			if c.tagged {
				tagged = append(tagged, c)
			}
		}
		if len(tagged) == 1 {
			out = append(out, tagged[0])
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].index < out[j].index })
	return out
}

func hasOpt(opts, name string) bool {
	for _, o := range strings.Split(opts, ",") {
		if o == name {
			return true
		}
	}
	return false
}

var tsIdent = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

func tsKey(name string) string {
	if tsIdent.MatchString(name) {
		return name
	}
	return strconv.Quote(name)
}
