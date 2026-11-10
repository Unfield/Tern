package native

import (
	"fmt"
	"sort"
	"strings"
)

type binding struct {
	expr    string // Go expression for reading the binding
	kind    string // var, let, const, func, param, class, import, catch
	assign  bool   // the expression is assignable in Go
	lazyImp *importRef
}

type importRef struct {
	from   *module
	shaken bool // the module was left out of the bundle
	name string // export name, or "*" for the namespace
	spec string
}

type scope struct {
	parent *scope
	names  map[string]*binding
	fn     *fnCtx
	top    bool // a module's top level, whose bindings are struct fields
}

func newScope(parent *scope, fn *fnCtx) *scope {
	return &scope{parent: parent, names: map[string]*binding{}, fn: fn}
}

func (s *scope) lookup(name string) *binding {
	for p := s; p != nil; p = p.parent {
		if b, ok := p.names[name]; ok {
			return b
		}
	}
	return nil
}

// declaredNames lists the names a declaration statement binds.
func declaredNames(n *Node) []string {
	switch n.Type {
	case "VariableDeclaration":
		var out []string
		for _, d := range n.List("declarations") {
			out = append(out, patternNames(d.Child("id"))...)
		}
		return out
	case "FunctionDeclaration", "ClassDeclaration":
		if id := n.Child("id"); id != nil {
			return []string{id.Str("name")}
		}
	}
	return nil
}

func patternNames(p *Node) []string {
	if p == nil {
		return nil
	}
	switch p.Type {
	case "Identifier":
		return []string{p.Str("name")}
	case "ObjectPattern":
		var out []string
		for _, prop := range p.List("properties") {
			if prop.Type == "RestElement" {
				out = append(out, patternNames(prop.Child("argument"))...)
			} else {
				out = append(out, patternNames(prop.Child("value"))...)
			}
		}
		return out
	case "ArrayPattern":
		var out []string
		for _, e := range p.List("elements") {
			out = append(out, patternNames(e)...)
		}
		return out
	case "RestElement":
		return patternNames(p.Child("argument"))
	case "AssignmentPattern":
		return patternNames(p.Child("left"))
	}
	return nil
}

// declareModule finds a module's top-level bindings and local exports.
func (c *Compiler) declareModule(m *module) {
	m.scope = newScope(nil, &fnCtx{this: "js.Undefined", module: true})
	m.scope.top = true
	m.exports = map[string]string{}
	field := func(name, kind string) {
		if _, ok := m.scope.names[name]; !ok {
			m.scope.names[name] = &binding{expr: m.goVar + "." + goIdent(name), kind: kind, assign: true}
		}
	}
	for _, n := range m.ast.List("body") {
		decl := n
		if n.Type == "ExportNamedDeclaration" || n.Type == "ExportDefaultDeclaration" {
			if d := n.Child("declaration"); d != nil {
				decl = d
			}
		}
		for _, name := range declaredNames(decl) {
			kind := "var"
			switch decl.Type {
			case "FunctionDeclaration":
				kind = "func"
			case "ClassDeclaration":
				kind = "class"
			case "VariableDeclaration":
				kind = decl.Str("kind")
			}
			field(name, kind)
		}
		switch n.Type {
		case "ImportDeclaration":
			spec := n.Child("source").Str("value")
			dep := c.resolveModule(m, spec)
			for _, s := range n.List("specifiers") {
				local := s.Child("local").Str("name")
				ref := &importRef{from: dep, spec: spec, shaken: c.isShaken(m, spec)}
				switch s.Type {
				case "ImportDefaultSpecifier":
					ref.name = "default"
				case "ImportNamespaceSpecifier":
					ref.name = "*"
				default:
					ref.name = moduleExportName(s.Child("imported"))
				}
				m.scope.names[local] = &binding{kind: "import", lazyImp: ref}
			}
		case "ExportDefaultDeclaration":
			d := n.Child("declaration")
			if (d.Type == "FunctionDeclaration" || d.Type == "ClassDeclaration") && d.Child("id") != nil {
				m.exports["default"] = m.goVar + "." + goIdent(d.Child("id").Str("name"))
			} else {
				field("*default*", "const")
				m.exports["default"] = m.goVar + "." + goIdent("*default*")
			}
		case "ExportNamedDeclaration":
			if d := n.Child("declaration"); d != nil {
				for _, name := range declaredNames(d) {
					m.exports[name] = m.goVar + "." + goIdent(name)
				}
			}
		}
	}
}

func moduleExportName(n *Node) string {
	if n.Type == "Literal" {
		return n.Str("value")
	}
	return n.Str("name")
}

// importExpr is the Go expression of an import binding.
func (c *Compiler) importExpr(ref *importRef) string {
	if ref.from == nil && ref.shaken {
		return fmt.Sprintf("r.TreeShaken(%s, %s)", quote(ref.spec), quote(ref.name))
	}
	if ref.from == nil {
		ns, ok := nativeModules[ref.spec]
		if !ok {
			// Reading the binding raises Unsupported, so only code that uses it falls back.
			return fmt.Sprintf("js.MissingImport(%s, %s)", quote(ref.spec), quote(ref.name))
		}
		if ref.name == "*" {
			return ns
		}
		return fmt.Sprintf("js.Get(%s, %s)", ns, quote(ref.name))
	}
	if ref.name == "*" {
		return fmt.Sprintf("r.ns%d()", ref.from.index)
	}
	c.resolveExports(ref.from, map[*module]bool{})
	if e, ok := ref.from.exports[ref.name]; ok {
		return e
	}
	return fmt.Sprintf("js.MissingExport(%s, %s)", quote(ref.from.id), quote(ref.name))
}

// resolveExports adds re-exports to a module's exports.
func (c *Compiler) resolveExports(m *module, seen map[*module]bool) {
	if m.resolved || seen[m] {
		return
	}
	seen[m] = true
	for _, n := range m.ast.List("body") {
		switch n.Type {
		case "ExportNamedDeclaration":
			if n.Child("declaration") != nil {
				continue
			}
			src := n.Child("source")
			for _, s := range n.List("specifiers") {
				local, exported := moduleExportName(s.Child("local")), moduleExportName(s.Child("exported"))
				if src == nil {
					if b, ok := m.scope.names[local]; ok {
						if b.lazyImp != nil {
							m.exports[exported] = c.importExpr(b.lazyImp)
						} else {
							m.exports[exported] = b.expr
						}
					}
					continue
				}
				spec := src.Str("value")
				dep := c.resolveModule(m, spec)
				m.exports[exported] = c.importExpr(&importRef{from: dep, name: local, spec: spec, shaken: c.isShaken(m, spec)})
			}
		case "ExportAllDeclaration":
			spec := n.Child("source").Str("value")
			dep := c.resolveModule(m, spec)
			if ex := n.Child("exported"); ex != nil {
				m.exports[moduleExportName(ex)] = c.importExpr(&importRef{from: dep, name: "*", spec: spec, shaken: c.isShaken(m, spec)})
				continue
			}
			if dep == nil {
				continue
			}
			c.resolveExports(dep, seen)
			m.starFrom = append(m.starFrom, dep)
		}
	}
	for _, dep := range m.starFrom {
		for name, e := range dep.exports {
			if _, ok := m.exports[name]; !ok && name != "default" {
				m.exports[name] = e
			}
		}
	}
	m.resolved = true
}

// compileModule writes a module's struct, its init and its namespace.
func (c *Compiler) compileModule(m *module) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// %s\n", m.id)
	fmt.Fprintf(&b, "type %s struct {\n", m.goType)
	var fields []string
	for name, bd := range m.scope.names {
		if bd.lazyImp == nil {
			fields = append(fields, name)
		}
	}
	sort.Strings(fields)
	for _, name := range fields {
		fmt.Fprintf(&b, "\t%s js.Value\n", goIdent(name))
	}
	b.WriteString("\tns *js.Object\n}\n\n")
	var zero strings.Builder
	fmt.Fprintf(&zero, "%s{", m.goType)
	for i, name := range fields {
		if i > 0 {
			zero.WriteString(", ")
		}
		fmt.Fprintf(&zero, "%s: js.Undefined", goIdent(name))
	}
	zero.WriteString("}")
	m.zero = zero.String()

	// Imports resolve now that every module's exports are known.
	for _, bd := range m.scope.names {
		if bd.lazyImp != nil {
			bd.expr = c.importExpr(bd.lazyImp)
		}
	}
	w := &writer{}
	w.indent = 1
	body := m.ast.List("body")
	c.hoistFunctions(w, m.scope, body)
	for _, n := range body {
		c.moduleItem(w, m, n)
	}
	fmt.Fprintf(&b, "func (r *realm) init%d() {\n%s}\n\n", m.index, w.String())

	names := make([]string, 0, len(m.exports))
	for name := range m.exports {
		names = append(names, name)
	}
	sort.Strings(names)
	fmt.Fprintf(&b, "func (r *realm) ns%d() js.Value {\n\tif %s.ns == nil {\n\t\t%s.ns = js.NewNamespace()\n", m.index, m.goVar, m.goVar)
	for _, name := range names {
		fmt.Fprintf(&b, "\t\tjs.ExportLive(%s.ns, %s, func() js.Value { return %s })\n", m.goVar, quote(name), m.exports[name])
	}
	fmt.Fprintf(&b, "\t}\n\treturn %s.ns\n}\n\n", m.goVar)
	return b.String()
}

// moduleItem compiles one top-level statement.
func (c *Compiler) moduleItem(w *writer, m *module, n *Node) {
	switch n.Type {
	case "ImportDeclaration", "ExportAllDeclaration":
		return
	case "ExportNamedDeclaration":
		if d := n.Child("declaration"); d != nil {
			c.statement(w, m.scope, d)
		}
		return
	case "ExportDefaultDeclaration":
		d := n.Child("declaration")
		if (d.Type == "FunctionDeclaration" || d.Type == "ClassDeclaration") && d.Child("id") != nil {
			c.statement(w, m.scope, d)
			return
		}
		if d.Type == "FunctionDeclaration" || d.Type == "ClassDeclaration" {
			// An anonymous default function or class is named "default".
			d.Type = map[string]string{"FunctionDeclaration": "FunctionExpression", "ClassDeclaration": "ClassExpression"}[d.Type]
		}
		w.line("%s.%s = %s", m.goVar, goIdent("*default*"), c.namedExpr(m.scope, d, "default"))
		return
	}
	c.statement(w, m.scope, n)
}
