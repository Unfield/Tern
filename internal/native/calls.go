package native

import (
	"fmt"
)

// args is a call's argument list, as Go call arguments after a leading comma.
func (c *Compiler) args(s *scope, list []*Node) string {
	spread := false
	for _, a := range list {
		if a.Type == "SpreadElement" {
			spread = true
		}
	}
	if !spread {
		out := ""
		for _, a := range list {
			out += ", " + c.expr(s, a)
		}
		return out
	}
	expr := "[]js.Value{}"
	for _, a := range list {
		if a.Type == "SpreadElement" {
			expr = fmt.Sprintf("append(%s, js.Spread(%s)...)", expr, c.expr(s, a.Child("argument")))
		} else {
			expr = fmt.Sprintf("append(%s, %s)", expr, c.expr(s, a))
		}
	}
	return ", " + expr + "..."
}

func (c *Compiler) call(s *scope, n *Node) string {
	callee := n.Child("callee")
	if callee.Type == "Super" {
		return c.superCall(s, n)
	}
	return c.callWith(s, callee, nil, true, n.List("arguments")...)
}

// callWith calls callee with either compiled args or argument nodes.
func (c *Compiler) callWith(s *scope, callee *Node, compiled []string, useNodes bool, nodes ...*Node) string {
	args := ""
	if useNodes {
		args = c.args(s, nodes)
	} else {
		for _, a := range compiled {
			args += ", " + a
		}
	}
	if callee.Type == "MemberExpression" && !callee.Bool("optional") {
		obj := callee.Child("object")
		prop := callee.Child("property")
		if obj.Type == "Super" {
			return fmt.Sprintf("js.Call(js.SuperGet(%s, %s, %s), %s%s)", s.fn.superProtoExpr(c, callee), c.memberKey(s, callee), c.thisExpr(s, callee), c.thisExpr(s, callee), args)
		}
		if prop.Type == "PrivateIdentifier" {
			t := c.temp("t")
			return iife(fmt.Sprintf("%s := %s\nreturn js.Call(js.PrivateGet(%s, %s), %s%s)\n", t, c.expr(s, obj), t, c.privateKey(s, prop), t, args))
		}
		if e, ns, ok := c.namespaceMember(s, callee); ok {
			return fmt.Sprintf("js.Call(%s, %s%s)", e, ns, args)
		}
		if !callee.Bool("computed") {
			return fmt.Sprintf("js.CallMethod(%s, %s%s)", c.expr(s, obj), quote(prop.Str("name")), args)
		}
		return fmt.Sprintf("js.CallMethodV(%s, %s%s)", c.expr(s, obj), c.expr(s, prop), args)
	}
	return fmt.Sprintf("js.Call(%s, js.Undefined%s)", c.expr(s, callee), args)
}

// namespaceMember resolves ns.name, where ns is a module namespace import
// and name one of its exports, to the export's binding: no lookup on the
// namespace object at run time. It also returns the namespace, the this
// of a call.
func (c *Compiler) namespaceMember(s *scope, n *Node) (expr, ns string, ok bool) {
	obj, prop := n.Child("object"), n.Child("property")
	if obj.Type != "Identifier" {
		return "", "", false
	}
	name := ""
	switch {
	case !n.Bool("computed") && prop.Type == "Identifier":
		name = prop.Str("name")
	case prop.Type == "Literal":
		if v, ok := literalValue(prop); ok {
			name, _ = v.(string)
		}
	}
	b := s.lookup(obj.Str("name"))
	if name == "" || b == nil || b.lazyImp == nil || b.lazyImp.name != "*" || b.lazyImp.from == nil {
		return "", "", false
	}
	from := b.lazyImp.from
	c.resolveExports(from, map[*module]bool{})
	e, ok := from.exports[name]
	if !ok {
		return "", "", false
	}
	return e, from.goVar + ".namespace()", true
}

// chain compiles an optional chain's element, returning early with
// undefined from the enclosing function where a link is nullish.
func (c *Compiler) chain(w *writer, s *scope, n *Node) string {
	inChain := func(x *Node) bool { return x.Type == "MemberExpression" || x.Type == "CallExpression" }
	switch n.Type {
	case "MemberExpression":
		obj := n.Child("object")
		var o string
		if inChain(obj) {
			o = c.chain(w, s, obj)
		} else {
			o = c.expr(s, obj)
		}
		t := c.temp("c")
		w.line("%s := %s", t, o)
		if n.Bool("optional") {
			w.open("if js.IsNullish(%s) {", t)
			w.line("return js.Undefined")
			w.close("")
		}
		if n.Child("property").Type == "PrivateIdentifier" {
			return fmt.Sprintf("js.PrivateGet(%s, %s)", t, c.privateKey(s, n.Child("property")))
		}
		return fmt.Sprintf("js.Get(%s, %s)", t, c.memberKey(s, n))
	case "CallExpression":
		callee := n.Child("callee")
		args := c.args(s, n.List("arguments"))
		if callee.Type == "MemberExpression" && callee.Child("object").Type != "Super" {
			obj := callee.Child("object")
			var o string
			if inChain(obj) {
				o = c.chain(w, s, obj)
			} else {
				o = c.expr(s, obj)
			}
			t, f := c.temp("c"), c.temp("f")
			w.line("%s := %s", t, o)
			if callee.Bool("optional") {
				w.open("if js.IsNullish(%s) {", t)
				w.line("return js.Undefined")
				w.close("")
			}
			if callee.Child("property").Type == "PrivateIdentifier" {
				w.line("%s := js.PrivateGet(%s, %s)", f, t, c.privateKey(s, callee.Child("property")))
			} else {
				w.line("%s := js.Get(%s, %s)", f, t, c.memberKey(s, callee))
			}
			if n.Bool("optional") {
				w.open("if js.IsNullish(%s) {", f)
				w.line("return js.Undefined")
				w.close("")
			}
			return fmt.Sprintf("js.Call(%s, %s%s)", f, t, args)
		}
		var fv string
		if inChain(callee) {
			fv = c.chain(w, s, callee)
		} else {
			fv = c.expr(s, callee)
		}
		f := c.temp("f")
		w.line("%s := %s", f, fv)
		if n.Bool("optional") {
			w.open("if js.IsNullish(%s) {", f)
			w.line("return js.Undefined")
			w.close("")
		}
		return fmt.Sprintf("js.Call(%s, js.Undefined%s)", f, args)
	}
	return c.expr(s, n)
}

// ref is something an assignment can write: a variable or a property.
type ref struct {
	read  string              // Go expression that reads it
	write func(string) string // Go statement that writes a value
}

// lvalue evaluates an assignment target's object and key once.
func (c *Compiler) lvalue(w *writer, s *scope, n *Node) *ref {
	switch n.Type {
	case "Identifier":
		name := n.Str("name")
		b := s.lookup(name)
		if b == nil {
			return &ref{
				read:  fmt.Sprintf("js.GlobalGet(%s)", quote(name)),
				write: func(v string) string { return fmt.Sprintf("js.GlobalSet(%s, %s)", quote(name), v) },
			}
		}
		if b.kind == "const" || b.kind == "import" || b.kind == "class-inner" {
			return &ref{read: b.expr, write: func(v string) string {
				return fmt.Sprintf("_ = %s\npanic(js.TypeErrorf(\"Assignment to constant variable.\"))", v)
			}}
		}
		return &ref{read: b.expr, write: func(v string) string { return fmt.Sprintf("%s = %s", b.expr, v) }}
	case "MemberExpression":
		obj := n.Child("object")
		if obj.Type == "Super" {
			this := c.thisExpr(s, n)
			key := c.temp("k")
			w.line("%s := %s", key, c.memberKey(s, n))
			proto := s.fn.superProtoExpr(c, n)
			return &ref{
				read:  fmt.Sprintf("js.SuperGet(%s, %s, %s)", proto, key, this),
				write: func(v string) string { return fmt.Sprintf("js.SuperSet(%s, %s, %s, %s)", proto, key, v, this) },
			}
		}
		o := c.temp("o")
		w.line("%s := %s", o, c.expr(s, obj))
		if n.Child("property").Type == "PrivateIdentifier" {
			key := c.privateKey(s, n.Child("property"))
			return &ref{
				read:  fmt.Sprintf("js.PrivateGet(%s, %s)", o, key),
				write: func(v string) string { return fmt.Sprintf("js.PrivateSet(%s, %s, %s)", o, key, v) },
			}
		}
		if !n.Bool("computed") {
			key := quote(n.Child("property").Str("name"))
			return &ref{
				read:  fmt.Sprintf("js.Get(%s, %s)", o, key),
				write: func(v string) string { return fmt.Sprintf("js.Set(%s, %s, %s)", o, key, v) },
			}
		}
		k := c.temp("k")
		w.line("%s := js.ToPropertyKey(%s)", k, c.expr(s, n.Child("property")))
		return &ref{
			read:  fmt.Sprintf("js.Get(%s, %s)", o, k),
			write: func(v string) string { return fmt.Sprintf("js.Set(%s, %s, %s)", o, k, v) },
		}
	}
	c.problem(n, "assigning to a %s", n.Type)
	return &ref{read: "js.Undefined", write: func(v string) string { return "_ = " + v }}
}

// assign compiles an assignment, writing statements to w; it returns the
// Go expression of the assigned value.
func (c *Compiler) assign(w *writer, s *scope, n *Node) string {
	op := n.Str("operator")
	left := n.Child("left")
	right := n.Child("right")
	if op == "=" && (left.Type == "ObjectPattern" || left.Type == "ArrayPattern") {
		v := c.temp("v")
		w.line("%s := %s", v, c.expr(s, right))
		c.assignPattern(w, s, left, v)
		return v
	}
	name := ""
	if left.Type == "Identifier" {
		name = left.Str("name")
	}
	r := c.lvalue(w, s, left)
	v := c.temp("v")
	switch op {
	case "=":
		w.line("%s := %s", v, c.namedExpr(s, right, name))
	case "||=", "&&=", "??=":
		cur := c.temp("cur")
		w.line("%s := %s", cur, r.read)
		test := map[string]string{"||=": "!js.Truthy(%s)", "&&=": "js.Truthy(%s)", "??=": "js.IsNullish(%s)"}[op]
		w.line("%s := %s", v, cur)
		w.open("if "+test+" {", cur)
		w.line("%s = %s", v, c.namedExpr(s, right, name))
		w.line("%s", r.write(v))
		w.close("")
		return v
	default:
		f := binaryOps[op[:len(op)-1]]
		if f == "" {
			c.problem(n, "the %s operator", op)
			f = "js.Add"
		}
		w.line("%s := %s(%s, %s)", v, f, r.read, c.expr(s, right))
	}
	w.line("%s", r.write(v))
	return v
}

// updateStatement compiles ++ or -- whose value isn't used.
func (c *Compiler) updateStatement(w *writer, s *scope, n *Node) {
	r := c.lvalue(w, s, n.Child("argument"))
	f := "js.Inc"
	if n.Str("operator") == "--" {
		f = "js.Dec"
	}
	w.line("%s", r.write(f+"("+r.read+")"))
}

// update compiles ++ and --; it returns the expression's value.
func (c *Compiler) update(s *scope, n *Node) string {
	w := &writer{}
	r := c.lvalue(w, s, n.Child("argument"))
	old, nv := c.temp("old"), c.temp("new")
	w.line("%s := js.ToNumber(%s)", old, r.read)
	if n.Str("operator") == "++" {
		w.line("%s := %s + 1", nv, old)
	} else {
		w.line("%s := %s - 1", nv, old)
	}
	w.line("%s", r.write("js.Box("+nv+")"))
	if n.Bool("prefix") {
		w.line("return %s", nv)
	} else {
		w.line("return %s", old)
	}
	return iife(w.String())
}

// superCall is super(...) in a derived constructor: it builds this.
func (c *Compiler) superCall(s *scope, n *Node) string {
	f := s.fn
	for f != nil && f.arrow {
		f = f.parent
	}
	if f == nil || f.superCtor == "" || f.thisVar == "" {
		c.problem(n, "super() outside a derived constructor")
		return "js.Fail(\"super()\")"
	}
	w := &writer{}
	w.line("if %s != nil { panic(js.ReferenceErrorf(\"Super constructor may only be called once\")) }", f.thisVar)
	w.line("%s = js.ConstructWith(%s, newTarget%s)", f.thisVar, f.superCtor, c.args(s, n.List("arguments")))
	if f.afterSuper != "" {
		w.line("%s(%s)", f.afterSuper, f.thisVar)
	}
	w.line("return %s", f.thisVar)
	return iife(w.String())
}
