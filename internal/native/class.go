package native

import (
	"fmt"
	"strconv"
)

// class compiles a class declaration or expression to a Go expression of
// its constructor.
func (c *Compiler) class(s *scope, n *Node, name string) string {
	id := strconv.Itoa(c.site())
	w := &writer{}
	cs := newScope(s, s.fn)
	ctor := "ctor" + id
	proto := "proto" + id
	parent := ""
	parentProto := "js.ObjectPrototype"
	if sc := n.Child("superClass"); sc != nil {
		parent = "parent" + id
		w.line("%s := %s", parent, c.expr(s, sc))
		parentProto = "parentProto" + id
		w.line("%s := js.ClassParentProto(%s)", parentProto, parent)
	}
	w.line("var %s *js.Object", ctor)
	w.line("%s := js.NewObject(%s)", proto, parentProto)
	if name != "" {
		cs.names[name] = &binding{expr: "js.Value(" + ctor + ")", kind: "class-inner"}
		if id := n.Child("id"); id != nil {
			cs.names[id.Str("name")] = &binding{expr: "js.Value(" + ctor + ")", kind: "class-inner"}
		}
	}
	members := n.Child("body").List("body")
	// Private names are per class evaluation.
	for _, m := range members {
		if k := m.Child("key"); k != nil && k.Type == "PrivateIdentifier" {
			pn := "#" + k.Str("name")
			if _, ok := cs.names[pn]; !ok {
				g := "priv" + strconv.Itoa(c.site())
				cs.names[pn] = &binding{expr: g, kind: "private"}
				w.line("%s := js.NewPrivateName(%s)", g, quote(pn))
			}
		}
	}

	// Instance fields and private methods, set up on each new instance.
	init := "init" + id
	iw := &writer{indent: 1}
	ifn := &fnCtx{parent: s.fn, this: "th" + id, superProto: parentProto}
	is := newScope(cs, ifn)
	var ctorNode *Node
	// Private methods and accessors come first: field initializers may
	// call them. Their functions are made once, here, and shared by every
	// instance, as the spec has it.
	for _, m := range members {
		if m.Type != "MethodDefinition" || m.Bool("static") || m.Child("key").Type != "PrivateIdentifier" {
			continue
		}
		keyName := "#" + m.Child("key").Str("name")
		fv := "pm" + strconv.Itoa(c.site())
		w.line("%s := %s", fv, c.methodFunction(cs, m.Child("value"), keyName, "", parentProto))
		switch m.Str("kind") {
		case "get":
			iw.line("js.PrivateAccessor(%s, %s, %s, nil)", ifn.this, cs.lookup(keyName).expr, fv)
		case "set":
			iw.line("js.PrivateAccessor(%s, %s, nil, %s)", ifn.this, cs.lookup(keyName).expr, fv)
		default:
			iw.line("js.PrivateDefine(%s, %s, %s)", ifn.this, cs.lookup(keyName).expr, fv)
		}
	}
	for _, m := range members {
		switch {
		case m.Type == "MethodDefinition" && m.Str("kind") == "constructor":
			ctorNode = m
		case m.Type == "PropertyDefinition" && !m.Bool("static"):
			key, keyName := c.classKey(is, iw, m)
			v := "js.Undefined"
			if val := m.Child("value"); val != nil {
				v = c.namedExpr(is, val, keyName)
			}
			if m.Child("key").Type == "PrivateIdentifier" {
				iw.line("js.PrivateDefine(%s, %s, %s)", ifn.this, cs.lookup(keyName).expr, v)
			} else {
				iw.line("js.DefineField(%s, %s, %s)", ifn.this, key, v)
			}
		}
	}
	w.line("%s := func(%s js.Value) {\n%s\t_ = %s\n}", init, ifn.this, iw.String(), ifn.this)

	// The constructor.
	length := 0
	cw := &writer{indent: 1}
	if parent == "" {
		cw.line("this := js.Value(js.OrdinaryCreateFromConstructor(newTarget, %s))", proto)
		cw.line("%s(this)", init)
		if ctorNode != nil {
			fn := c.functionWith(cs, ctorNode.Child("value"), name, func(f *fnCtx) {
				f.superProto = parentProto
				if parentProto == "" {
					f.superProto = "js.ProtoOf(\"\")"
				}
				f.classCtor = true
			})
			length = len(ctorNode.Child("value").List("params"))
			cw.line("res := js.Call(%s, this, args...)", fn)
			cw.line("if o, ok := res.(*js.Object); ok { return o }")
		}
		cw.line("return this")
	} else {
		if ctorNode == nil {
			cw.line("this := js.ConstructWith(%s, newTarget, args...)", parent)
			cw.line("%s(this)", init)
			cw.line("return this")
		} else {
			length = len(ctorNode.Child("value").List("params"))
			th := "self" + id
			cw.line("var %s js.Value", th)
			body := c.functionWith(cs, ctorNode.Child("value"), name, func(f *fnCtx) {
				f.thisVar = th
				f.superCtor = parent
				f.superProto = parentProto
				f.afterSuper = init
				f.classCtor = true
			})
			cw.line("res := js.Call(%s, js.Undefined, args...)", body)
			cw.line("if o, ok := res.(*js.Object); ok { return o }")
			cw.line("return js.CheckThis(%s)", th)
		}
	}
	w.line("%s = js.NewConstructor(%s, %d, nil, func(args []js.Value, newTarget *js.Object) js.Value {\n%s}, %s)", ctor, quote(name), length, cw.String(), proto)
	w.line("js.AsObject(%s).SetInfo(&%s)", ctor, c.fnInfo(name, length, n))
	if parent != "" {
		w.line("js.InheritStatics(%s, %s)", ctor, parent)
	}

	// Methods, accessors and static members, in order.
	for _, m := range members {
		static := m.Bool("static")
		home := proto
		superBase := parentProto
		if static {
			home = ctor
			superBase = "js.ProtoOf(" + ctor + ")"
		}
		switch m.Type {
		case "MethodDefinition":
			if m.Str("kind") == "constructor" || (!static && m.Child("key").Type == "PrivateIdentifier") {
				continue
			}
			key, keyName := c.classKey(cs, w, m)
			if m.Child("key").Type == "PrivateIdentifier" {
				fn := c.methodFunction(cs, m.Child("value"), keyName, "", superBase)
				w.line("js.PrivateDefine(%s, %s, %s)", ctor, cs.lookup(keyName).expr, fn)
				continue
			}
			label := keyName
			switch m.Str("kind") {
			case "get", "set":
				label = m.Str("kind") + " " + keyName
			}
			fn := c.methodFunction(cs, m.Child("value"), label, "", superBase)
			switch m.Str("kind") {
			case "get":
				w.line("%s.DefineAccessor(%s, js.AsObject(%s), nil, false)", home, key, fn)
			case "set":
				w.line("%s.DefineAccessor(%s, nil, js.AsObject(%s), false)", home, key, fn)
			default:
				w.line("%s.DefineHidden(%s, %s)", home, key, fn)
			}
		case "PropertyDefinition":
			if !static {
				continue
			}
			sf := &fnCtx{parent: s.fn, this: "js.Value(" + ctor + ")", superProto: superBase}
			ss := newScope(cs, sf)
			key, keyName := c.classKey(ss, w, m)
			v := "js.Undefined"
			if val := m.Child("value"); val != nil {
				v = c.namedExpr(ss, val, keyName)
			}
			if m.Child("key").Type == "PrivateIdentifier" {
				w.line("js.PrivateDefine(%s, %s, %s)", ctor, cs.lookup(keyName).expr, v)
			} else {
				w.line("js.DefineField(%s, %s, %s)", ctor, key, v)
			}
		case "StaticBlock":
			sf := &fnCtx{parent: s.fn, this: "js.Value(" + ctor + ")", superProto: superBase}
			sb := newScope(cs, sf)
			w.open("func() {")
			c.statements(w, sb, m.List("body"))
			w.close("()")
		default:
			c.problem(m, "the class member %s", m.Type)
		}
	}
	w.line("return %s", ctor)
	return iife(w.String())
}

// classKey is a class member's key, as a Go PropKey, and its name for
// function names.
func (c *Compiler) classKey(s *scope, w *writer, m *Node) (string, string) {
	k := m.Child("key")
	if k.Type == "PrivateIdentifier" {
		return "", "#" + k.Str("name")
	}
	key, name := propKey(c, s, m)
	if m.Bool("computed") {
		t := c.temp("k")
		w.line("%s := %s", t, key)
		return t, ""
	}
	return key, name
}

// superProtoExpr is where super.x looks.
func (f *fnCtx) superProtoExpr(c *Compiler, n *Node) string {
	for g := f; g != nil; g = g.parent {
		if g.superProto != "" {
			return g.superProto
		}
		if !g.arrow {
			break
		}
	}
	c.problem(n, "super outside a method")
	return "js.ObjectPrototype"
}

var _ = fmt.Sprintf
