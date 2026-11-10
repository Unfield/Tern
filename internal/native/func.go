package native

import (
	"fmt"
	"strconv"
)

// varNames lists the var declarations of a function body, not looking into
// nested functions.
func varNames(body []*Node) []string {
	var out []string
	var walk func(n *Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		switch n.Type {
		case "FunctionDeclaration", "FunctionExpression", "ArrowFunctionExpression", "ClassDeclaration", "ClassExpression":
			return
		case "VariableDeclaration":
			if n.Str("kind") == "var" {
				out = append(out, declaredNames(n)...)
			}
		}
		for _, key := range []string{"body", "consequent", "alternate", "block", "handler", "finalizer", "init", "left"} {
			if ch := n.Child(key); ch != nil {
				walk(ch)
			}
			for _, ch := range n.List(key) {
				walk(ch)
			}
		}
		for _, cs := range n.List("cases") {
			for _, st := range cs.List("consequent") {
				walk(st)
			}
		}
	}
	for _, n := range body {
		walk(n)
	}
	return out
}

// function compiles a function or arrow to a Go expression of a function object.
func (c *Compiler) function(s *scope, n *Node, name string) string {
	return c.functionWith(s, n, name, func(*fnCtx) {})
}

// methodFunction compiles an object literal's method: super in it reads
// the object's prototype.
func (c *Compiler) methodFunction(s *scope, n *Node, name, home, superProto string) string {
	return c.functionWith(s, n, name, func(f *fnCtx) {
		if superProto != "" {
			f.superProto = superProto
		} else {
			f.superProto = "js.ProtoOf(" + home + ")"
		}
	})
}

func (c *Compiler) functionWith(s *scope, n *Node, name string, setup func(*fnCtx)) string {
	arrow := n.Type == "ArrowFunctionExpression"
	if n.Bool("async") {
		kind := "an async function"
		if n.Bool("generator") {
			kind = "an async generator"
		}
		c.problem(n, "%s", kind)
		return fmt.Sprintf("js.Value(js.NewFunction(%s, 0, func(js.Value, []js.Value) js.Value { panic(js.NotSupported(%s)) }))", quote(name), quote(kind))
	}
	id := strconv.Itoa(c.site())
	f := &fnCtx{parent: s.fn, arrow: arrow, args: "args" + id}
	if !arrow {
		f.this = "this" + id
		f.argumentsVar = "arguments" + id
	}
	if n.Bool("generator") {
		f.generator = "gen" + id
	}
	setup(f)
	outer := s
	selfName := ""
	if n.Type == "FunctionExpression" && n.Child("id") != nil {
		// A named function expression sees its own name.
		selfName = "self" + id
		outer = newScope(s, s.fn)
		outer.names[n.Child("id").Str("name")] = &binding{expr: selfName, kind: "const"}
	}
	fs := newScope(outer, f)
	body := &writer{indent: 1}
	params := n.List("params")
	length := 0
	counting := true
	for i, p := range params {
		switch p.Type {
		case "Identifier":
			if counting {
				length++
			}
			g := c.local(p.Str("name"))
			fs.names[p.Str("name")] = &binding{expr: g, kind: "param", assign: true}
			body.line("var %s js.Value = js.Arg(%s, %d)", g, f.args, i)
			body.line("_ = %s", g)
		case "AssignmentPattern":
			counting = false
			left := p.Child("left")
			src := c.temp("p")
			body.line("%s := js.Arg(%s, %d)", src, f.args, i)
			body.open("if %s == js.Undefined {", src)
			// Defaults see earlier parameters.
			body.line("%s = %s", src, c.namedExpr(fs, p.Child("right"), simpleName(left)))
			body.close("")
			c.declarePatternLocals(body, fs, left)
			c.bindPattern(body, fs, left, src, true)
		case "RestElement":
			counting = false
			src := c.temp("rest")
			body.line("%s := js.Value(js.Rest(%s, %d))", src, f.args, i)
			c.declarePatternLocals(body, fs, p.Child("argument"))
			c.bindPattern(body, fs, p.Child("argument"), src, true)
		default:
			if counting {
				length++
			}
			src := c.temp("p")
			body.line("%s := js.Arg(%s, %d)", src, f.args, i)
			c.declarePatternLocals(body, fs, p)
			c.bindPattern(body, fs, p, src, true)
		}
	}
	// The body runs in its own scope, after the parameters'.
	bs := newScope(fs, f)
	main := &writer{indent: 1}
	b := n.Child("body")
	if b.Type == "BlockStatement" {
		for _, v := range varNames(b.List("body")) {
			if fs.names[v] == nil && bs.names[v] == nil { // a var may repeat a parameter
				g := c.local(v)
				bs.names[v] = &binding{expr: g, kind: "var", assign: true}
				main.line("var %s js.Value = js.Undefined", g)
				main.line("_ = %s", g)
			}
		}
		c.statements(main, bs, b.List("body"))
		main.line("return js.Undefined")
	} else {
		main.line("return %s", c.expr(bs, b))
	}
	thisParam := "_"
	if !arrow {
		thisParam = f.this
	}
	prelude := ""
	if f.usesArguments {
		prelude = fmt.Sprintf("\t%s := js.Value(js.NewArguments(%s))\n\t_ = %s\n", f.argumentsVar, f.args, f.argumentsVar)
	}
	if f.generator != "" {
		// The parameters are bound when the generator function is called,
		// the body runs on the first next().
		fn := fmt.Sprintf("js.NewGeneratorFunction(%s, %d, func(%s js.Value, %s []js.Value) func(*js.Gen) js.Value {\n%s%s\treturn func(%s *js.Gen) js.Value {\n%s\t}\n})",
			quote(name), length, thisParam, f.args, prelude, body.String(), f.generator, main.String())
		if selfName != "" {
			return iife(fmt.Sprintf("var %s js.Value\n%s = %s\nreturn %s\n", selfName, selfName, fn, selfName))
		}
		return "js.Value(" + fn + ")"
	}
	fn := fmt.Sprintf("js.NewFunction(%s, %d, func(%s js.Value, %s []js.Value) js.Value {\n%s%s%s})", quote(name), length, thisParam, f.args, prelude, body.String(), main.String())
	if !arrow {
		fn = fmt.Sprintf("js.NonArrow(%s)", fn)
	}
	if selfName != "" {
		return iife(fmt.Sprintf("var %s js.Value\n%s = %s\nreturn %s\n", selfName, selfName, fn, selfName))
	}
	return "js.Value(" + fn + ")"
}

func simpleName(p *Node) string {
	if p.Type == "Identifier" {
		return p.Str("name")
	}
	return ""
}
