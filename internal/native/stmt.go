package native

import (
	"fmt"
	"strconv"
	"strings"
)

// hoistFunctions creates a scope's function declarations before anything
// else runs, and declares its let, const and class bindings.
func (c *Compiler) hoistFunctions(w *writer, s *scope, body []*Node) {
	for _, n := range body {
		d := n
		if n.Type == "ExportNamedDeclaration" || n.Type == "ExportDefaultDeclaration" {
			if x := n.Child("declaration"); x != nil {
				d = x
			}
		}
		if d.Type == "FunctionDeclaration" && d.Child("id") != nil {
			name := d.Child("id").Str("name")
			b := s.lookup(name)
			if s.top {
				if e, ok := c.intrinsicFor(c.cur, name, d); ok {
					w.line("%s = %s", b.expr, e)
					continue
				}
			}
			w.line("%s = %s", b.expr, c.function(s, d, name))
		}
	}
}

// declareBlock declares a block's lexical bindings as Go locals (in a
// function) before its statements run.
func (c *Compiler) declareBlock(w *writer, s *scope, body []*Node) {
	for _, n := range body {
		if n.Type != "VariableDeclaration" && n.Type != "ClassDeclaration" && n.Type != "FunctionDeclaration" {
			continue
		}
		if n.Type == "VariableDeclaration" && n.Str("kind") == "var" {
			continue // hoisted to the function
		}
		for _, name := range declaredNames(n) {
			if _, ok := s.names[name]; ok {
				continue
			}
			g := c.local(name)
			s.names[name] = &binding{expr: g, kind: n.Type, assign: true}
			w.line("var %s js.Value = js.Undefined", g)
			w.line("_ = %s", g)
		}
	}
}

// local is a unique Go name for a JavaScript local.
func (c *Compiler) local(name string) string {
	c.tmp++
	return goIdent(name) + "_" + strconv.Itoa(c.tmp)
}

func (c *Compiler) statements(w *writer, s *scope, body []*Node) {
	if !s.top {
		c.declareBlock(w, s, body)
		c.hoistFunctions(w, s, body)
	}
	for _, n := range body {
		c.statement(w, s, n)
	}
}

func (c *Compiler) statement(w *writer, s *scope, n *Node) {
	switch n.Type {
	case "EmptyStatement", "DebuggerStatement":
	case "ExpressionStatement":
		c.exprStatement(w, s, n.Child("expression"))
	case "VariableDeclaration":
		for _, d := range n.List("declarations") {
			init := d.Child("init")
			if init == nil {
				if n.Str("kind") != "var" {
					for _, name := range patternNames(d.Child("id")) {
						w.line("%s = js.Undefined", s.lookup(name).expr)
					}
				}
				continue
			}
			id := d.Child("id")
			if id.Type == "Identifier" {
				w.line("%s = %s", s.lookup(id.Str("name")).expr, c.namedExpr(s, init, id.Str("name")))
				continue
			}
			tmp := c.temp("d")
			w.line("%s := %s", tmp, c.expr(s, init))
			c.bindPattern(w, s, id, tmp, false)
		}
	case "FunctionDeclaration":
		// Hoisted with the scope.
	case "ClassDeclaration":
		name := n.Child("id").Str("name")
		w.line("%s = %s", s.lookup(name).expr, c.class(s, n, name))
	case "ReturnStatement":
		v := "js.Undefined"
		if a := n.Child("argument"); a != nil {
			v = c.expr(s, a)
		}
		if s.fn.module {
			c.problem(n, "return at the top level")
			return
		}
		if s.fn.tryDepth > 0 {
			w.line("return js.CtlReturn, %s", v)
		} else {
			w.line("return %s", v)
		}
	case "IfStatement":
		w.open("if %s {", c.cond(s, n.Child("test")))
		c.block(w, s, n.Child("consequent"))
		if alt := n.Child("alternate"); alt != nil {
			w.indent--
			w.open("} else {")
			c.block(w, s, alt)
		}
		w.close("")
	case "BlockStatement":
		w.open("{")
		c.statements(w, newScope(s, s.fn), n.List("body"))
		w.close("")
	case "ThrowStatement":
		w.line("panic(js.ThrowValue(%s))", c.expr(s, n.Child("argument")))
	case "TryStatement":
		c.tryStatement(w, s, n)
	case "WhileStatement", "DoWhileStatement", "ForStatement", "ForOfStatement", "ForInStatement":
		c.loop(w, s, n, nil)
	case "LabeledStatement":
		var labels []string
		body := n
		for body.Type == "LabeledStatement" {
			labels = append(labels, body.Child("label").Str("name"))
			body = body.Child("body")
		}
		switch body.Type {
		case "WhileStatement", "DoWhileStatement", "ForStatement", "ForOfStatement", "ForInStatement":
			c.loop(w, s, body, labels)
		case "SwitchStatement":
			c.switchStatement(w, s, body, labels)
		default:
			// A labeled block: a loop that runs once, so break can leave it.
			t := &target{label: c.temp("L"), jsLabels: labels}
			s.fn.push(t)
			inner := w.sub()
			inner.indent++
			c.statement(inner, s, body)
			inner.line("break %s", t.label)
			s.fn.pop()
			w.line("%s:", t.label)
			w.open("for {")
			w.raw(inner.String())
			w.close("")
		}
	case "SwitchStatement":
		c.switchStatement(w, s, n, nil)
	case "BreakStatement", "ContinueStatement":
		c.jump(w, s, n)
	default:
		c.problem(n, "the %s statement", n.Type)
		w.line("panic(js.NotSupported(%s))", quote(n.Type))
	}
}

// block compiles a statement that may or may not be a block, in a new scope.
func (c *Compiler) block(w *writer, s *scope, n *Node) {
	inner := newScope(s, s.fn)
	if n.Type == "BlockStatement" {
		c.statements(w, inner, n.List("body"))
		return
	}
	c.statements(w, inner, []*Node{n})
}

func (c *Compiler) jump(w *writer, s *scope, n *Node) {
	isBreak := n.Type == "BreakStatement"
	want := ""
	if l := n.Child("label"); l != nil {
		want = l.Str("name")
	}
	var t *target
	for i := len(s.fn.targets) - 1; i >= 0; i-- {
		cand := s.fn.targets[i]
		if want != "" {
			if contains(cand.jsLabels, want) {
				t = cand
				break
			}
			continue
		}
		if isBreak && (cand.loop || cand.label != "" && cand.jsLabels == nil) || !isBreak && cand.loop {
			t = cand
			break
		}
	}
	if t == nil {
		c.problem(n, "a %s with no target", n.Type)
		return
	}
	if !isBreak && !t.loop {
		c.problem(n, "continue to a label that isn't a loop")
		return
	}
	t.used = true
	if s.fn.tryDepth > t.tryDepth {
		code := "js.CtlBreak"
		if !isBreak {
			code = "js.CtlContinue"
		}
		if t.id == 0 {
			c.tmp++
			t.id = c.tmp
		}
		w.line("return %s, %d.0", code, t.id)
		return
	}
	if isBreak {
		w.line("break %s", t.label)
	} else {
		w.line("continue %s", t.label)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// loop compiles while, do…while, for, for…of and for…in.
func (c *Compiler) loop(w *writer, s *scope, n *Node, labels []string) {
	t := &target{label: c.temp("L"), jsLabels: labels, loop: true}
	s.fn.push(t)
	defer s.fn.pop()
	ls := newScope(s, s.fn) // for the loop's own let/const
	out := w.sub()
	// Go wants a label right on its for statement, after any setup; it is
	// known to be needed only once the body is compiled.
	mark := "//label:" + t.label
	switch n.Type {
	case "WhileStatement":
		out.line("%s", mark)
		out.open("for %s {", c.cond(ls, n.Child("test")))
		out.line("r.Tick()")
		c.block(out, ls, n.Child("body"))
		out.close("")
	case "DoWhileStatement":
		first := c.temp("first")
		out.line("%s := true", first)
		out.line("%s", mark)
		out.open("for ; %s || %s; %s = false {", first, c.cond(ls, n.Child("test")), first)
		out.line("r.Tick()")
		c.block(out, ls, n.Child("body"))
		out.close("")
	case "ForStatement":
		init := n.Child("init")
		test := "true"
		post := ""
		header := func() {
			if t := n.Child("test"); t != nil {
				test = c.cond(ls, t)
			}
			if u := n.Child("update"); u != nil {
				uw := &writer{}
				c.exprStatement(uw, ls, u)
				if code := strings.TrimSpace(uw.String()); !strings.Contains(code, "\n") && !strings.Contains(code, ":=") {
					post = code // one simple statement, as i++ is
				} else {
					post = "func() {\n" + uw.String() + "}()"
				}
			}
		}
		// for (let i = …; …; …) gets a binding per iteration, so closures in
		// the body keep their iteration's value: Go's for-loop variables do
		// exactly that.
		if init != nil && init.Type == "VariableDeclaration" && init.Str("kind") != "var" && simpleDeclarators(init) {
			var names, values []string
			for _, d := range init.List("declarations") {
				name := d.Child("id").Str("name")
				g := c.local(name)
				v := "js.Value(js.Undefined)"
				if e := d.Child("init"); e != nil {
					v = "js.Value(" + c.namedExpr(ls, e, name) + ")"
				}
				ls.names[name] = &binding{expr: g, kind: "let", assign: true}
				names = append(names, g)
				values = append(values, v)
			}
			header()
			initExpr := join(values)
			if len(names) > 1 {
				// Later declarators may read earlier ones, and Go evaluates
				// all of a for's initializers first: evaluate them in order.
				types := strings.TrimSuffix(strings.Repeat("js.Value, ", len(names)), ", ")
				var b strings.Builder
				fmt.Fprintf(&b, "func() (%s) {\nvar %s js.Value\n", types, join(names))
				for i, g := range names {
					fmt.Fprintf(&b, "%s = %s\n", g, values[i])
				}
				fmt.Fprintf(&b, "return %s\n}()", join(names))
				initExpr = b.String()
			}
			out.line("%s", mark)
			out.open("for %s := %s; %s; %s {", join(names), initExpr, test, post)
			out.line("r.Tick()")
			for _, g := range names {
				out.line("_ = %s", g)
			}
			c.block(out, ls, n.Child("body"))
			out.close("")
			break
		}
		out.open("{")
		if init != nil {
			if init.Type == "VariableDeclaration" {
				if init.Str("kind") != "var" {
					c.declareBlock(out, ls, []*Node{init})
				}
				c.statement(out, ls, init)
			} else {
				c.exprStatement(out, ls, init)
			}
		}
		header()
		out.line("%s", mark)
		out.open("for ; %s; %s {", test, post)
		out.line("r.Tick()")
		c.block(out, ls, n.Child("body"))
		out.close("")
		out.close("")
	case "ForOfStatement", "ForInStatement":
		out.open("{")
		next := c.temp("next")
		if n.Type == "ForOfStatement" {
			if n.Bool("await") {
				c.problem(n, "for await")
			}
			out.line("%s := js.Iterate(%s)", next, c.expr(ls, n.Child("right")))
		} else {
			out.line("%s := js.ForInKeys(%s)", next, c.expr(ls, n.Child("right")))
		}
		out.line("%s", mark)
		out.open("for {")
		out.line("r.Tick()")
		v := c.temp("x")
		out.line("%s, ok := %s()", v, next)
		out.open("if !ok {")
		out.line("break")
		out.close("")
		body := newScope(ls, ls.fn)
		left := n.Child("left")
		if left.Type == "VariableDeclaration" {
			c.declareBlock(out, body, []*Node{left})
			if left.Str("kind") == "var" {
				c.declareVarsIn(out, body, left)
			}
			c.bindPattern(out, body, left.List("declarations")[0].Child("id"), v, false)
		} else {
			c.assignPattern(out, body, left, v)
		}
		c.block(out, body, n.Child("body"))
		out.close("")
		out.close("")
	}
	lines := strings.Split(out.String(), "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == mark {
			if t.used {
				lines[i] = strings.Replace(l, mark, t.label+":", 1)
			} else {
				lines = append(lines[:i], lines[i+1:]...)
			}
			break
		}
	}
	w.raw(strings.Join(lines, "\n"))
}

func (c *Compiler) declareVarsIn(w *writer, s *scope, decl *Node) {
	for _, name := range declaredNames(decl) {
		if s.lookup(name) == nil {
			g := c.local(name)
			s.names[name] = &binding{expr: g, kind: "var", assign: true}
			w.line("var %s js.Value = js.Undefined", g)
			w.line("_ = %s", g)
		}
	}
}

func simpleDeclarators(decl *Node) bool {
	for _, d := range decl.List("declarations") {
		if d.Child("id").Type != "Identifier" {
			return false
		}
	}
	return true
}

func join(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

func (c *Compiler) switchStatement(w *writer, s *scope, n *Node, labels []string) {
	t := &target{label: c.temp("L"), jsLabels: labels}
	s.fn.push(t)
	defer s.fn.pop()
	disc := c.temp("sw")
	out := w.sub()
	out.open("{")
	out.line("%s := %s", disc, c.expr(s, n.Child("discriminant")))
	inner := newScope(s, s.fn)
	cases := n.List("cases")
	var all []*Node
	for _, cs := range cases {
		all = append(all, cs.List("consequent")...)
	}
	c.declareBlock(out, inner, all)
	c.hoistFunctions(out, inner, all)
	// Find the first matching case, then fall through as JavaScript does.
	start := c.temp("case")
	out.line("%s := -1", start)
	def := -1
	for i, cs := range cases {
		if cs.Child("test") == nil {
			def = i
			continue
		}
		out.open("if %s < 0 && js.StrictEquals(%s, %s) {", start, disc, c.expr(inner, cs.Child("test")))
		out.line("%s = %d", start, i)
		out.close("")
	}
	if def >= 0 {
		out.open("if %s < 0 {", start)
		out.line("%s = %d", start, def)
		out.close("")
	}
	sw := out.sub()
	sw.open("switch {")
	for i, cs := range cases {
		// Go's fallthrough enters the next case unconditionally, as JavaScript does.
		sw.open("case %s == %d:", start, i)
		for _, st := range cs.List("consequent") {
			c.statement(sw, inner, st)
		}
		if i+1 < len(cases) {
			sw.line("fallthrough")
		}
		sw.indent--
	}
	sw.line("}")
	if t.used {
		out.line("%s:", t.label)
	}
	out.raw(sw.String())
	out.close("")
	w.raw(out.String())
}

// tryStatement compiles try…catch…finally with closures; control that
// leaves the try comes back as a code and is replayed here.
func (c *Compiler) tryStatement(w *writer, s *scope, n *Node) {
	f := s.fn
	f.tryDepth++
	body := &writer{indent: 2}
	c.block(body, s, n.Child("block"))
	body.line("return js.CtlNone, nil")
	catch := "nil"
	if h := n.Child("handler"); h != nil {
		cw := &writer{indent: 2}
		cs := newScope(s, f)
		e := c.temp("e")
		if p := h.Child("param"); p != nil {
			if p.Type == "Identifier" {
				g := c.local(p.Str("name"))
				cs.names[p.Str("name")] = &binding{expr: g, kind: "catch", assign: true}
				cw.line("var %s js.Value = %s", g, e)
				cw.line("_ = %s", g)
			} else {
				c.declarePatternLocals(cw, cs, p)
				c.bindPattern(cw, cs, p, e, true)
			}
		}
		c.block(cw, cs, h.Child("body"))
		cw.line("return js.CtlNone, nil")
		catch = fmt.Sprintf("func(%s js.Value) (int, js.Value) {\n%s\t}", e, cw.String())
	}
	finally := "nil"
	if fin := n.Child("finalizer"); fin != nil {
		fw := &writer{indent: 2}
		c.block(fw, s, fin)
		fw.line("return js.CtlNone, nil")
		finally = fmt.Sprintf("func() (int, js.Value) {\n%s\t}", fw.String())
	}
	f.tryDepth--
	ctl, val := c.temp("ctl"), c.temp("cv")
	w.open("if %s, %s := js.Try(func() (int, js.Value) {", ctl, val)
	w.raw(body.String())
	w.indent--
	w.line("}, %s, %s); %s != js.CtlNone {", catch, finally, ctl)
	w.indent++
	c.replayControl(w, s, ctl, val)
	w.close("")
}

// replayControl continues a return, break or continue that left a try.
func (c *Compiler) replayControl(w *writer, s *scope, ctl, val string) {
	f := s.fn
	w.line("_ = %s", val)
	w.open("switch %s {", ctl)
	switch {
	case f.tryDepth > 0:
		w.line("case js.CtlReturn:")
		w.line("\treturn %s, %s", ctl, val)
	case !f.module: // a module's top level has nothing to return from
		w.line("case js.CtlReturn:")
		w.line("\treturn %s", val)
	}
	for _, t := range f.targets {
		if t.id == 0 {
			continue
		}
		for _, code := range []string{"js.CtlBreak", "js.CtlContinue"} {
			if code == "js.CtlContinue" && !t.loop {
				continue
			}
			w.line("case %s:", code)
			w.open("\tif %s == %d.0 {", val, t.id)
			switch {
			case f.tryDepth > t.tryDepth:
				w.line("return %s, %s", ctl, val)
			case code == "js.CtlBreak":
				w.line("break %s", t.label)
			default:
				w.line("continue %s", t.label)
			}
			w.close("")
		}
	}
	w.close("")
}
