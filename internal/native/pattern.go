package native

import "fmt"

// declarePatternLocals declares Go locals for the names a pattern binds.
func (c *Compiler) declarePatternLocals(w *writer, s *scope, p *Node) {
	for _, name := range patternNames(p) {
		if _, ok := s.names[name]; ok {
			continue
		}
		g := c.local(name)
		s.names[name] = &binding{expr: g, kind: "let", assign: true}
		w.line("var %s js.Value = js.Undefined", g)
		w.line("_ = %s", g)
	}
}

// bindPattern binds a declaration's pattern from the Go expression src
// (a variable); the names are already declared.
func (c *Compiler) bindPattern(w *writer, s *scope, p *Node, src string, _ bool) {
	c.destructure(w, s, p, src, func(target *Node, v string) {
		name := target.Str("name")
		if b := s.lookup(name); b != nil {
			w.line("%s = %s", b.expr, v)
			return
		}
		c.problem(target, "binding %s before it is declared", name)
	})
}

// assignPattern assigns a destructuring pattern's targets, which may be
// any assignable expressions.
func (c *Compiler) assignPattern(w *writer, s *scope, p *Node, src string) {
	c.destructure(w, s, p, src, func(target *Node, v string) {
		r := c.lvalue(w, s, target)
		w.line("%s", r.write(v))
	})
}

func (c *Compiler) destructure(w *writer, s *scope, p *Node, src string, set func(*Node, string)) {
	switch p.Type {
	case "Identifier", "MemberExpression":
		set(p, src)
	case "AssignmentPattern":
		v := c.temp("v")
		w.line("%s := %s", v, src)
		w.open("if %s == js.Undefined {", v)
		w.line("%s = %s", v, c.namedExpr(s, p.Child("right"), simpleName(p.Child("left"))))
		w.close("")
		c.destructure(w, s, p.Child("left"), v, set)
	case "ObjectPattern":
		w.line("js.RequireObjectCoercible(%s)", src)
		var used []string
		for _, prop := range p.List("properties") {
			if prop.Type == "RestElement" {
				rest := c.temp("rest")
				w.line("%s := js.Value(js.CopyDataPropertiesExcluding(%s, []js.PropKey{%s}))", rest, src, join(used))
				c.destructure(w, s, prop.Child("argument"), rest, set)
				continue
			}
			key, _ := propKey(c, s, prop)
			if prop.Bool("computed") {
				k := c.temp("k")
				w.line("%s := %s", k, key)
				key = k
			}
			used = append(used, key)
			v := c.temp("v")
			w.line("%s := js.Get(%s, %s)", v, src, key)
			c.destructure(w, s, prop.Child("value"), v, set)
		}
	case "ArrayPattern":
		next := c.temp("it")
		w.line("%s := js.Iterate(%s)", next, src)
		for _, el := range p.List("elements") {
			if el == nil {
				w.line("%s()", next)
				continue
			}
			if el.Type == "RestElement" {
				rest := c.temp("rest")
				w.line("%s := js.Value(js.Remaining(%s))", rest, next)
				c.destructure(w, s, el.Child("argument"), rest, set)
				continue
			}
			v := c.temp("v")
			w.line("%s, _ := %s()", v, next)
			c.destructure(w, s, el, v, set)
		}
	default:
		c.problem(p, "the %s pattern", p.Type)
		w.line("panic(js.NotSupported(%s))", quote(fmt.Sprint(p.Type)))
	}
}
