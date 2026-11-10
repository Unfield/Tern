package native

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/Unfield/Tern/native/js"
)

var binaryOps = map[string]string{
	"+": "js.Add", "-": "js.Sub", "*": "js.Mul", "/": "js.Div", "%": "js.Mod", "**": "js.Pow",
	"&": "js.BitAnd", "|": "js.BitOr", "^": "js.BitXor", "<<": "js.Shl", ">>": "js.Shr", ">>>": "js.UShr",
}

var compareOps = map[string]string{
	"==": "js.LooseEquals", "===": "js.StrictEquals", "<": "js.Less", ">": "js.Greater", "<=": "js.LessEq", ">=": "js.GreaterEq",
	"in": "js.In", "instanceof": "js.InstanceOf",
}

// literalValue decodes a Literal's value exactly, keeping lone surrogates.
func literalValue(n *Node) (js.Value, bool) {
	raw := n.Raw("value")
	if n.Has("regex") || n.Has("bigint") {
		return nil, false
	}
	if string(raw) == "null" || raw == nil {
		return js.Null, true
	}
	v, err := js.ParseJSON(string(raw))
	if err != nil {
		return nil, false
	}
	return v, true
}

func goNumber(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return "js.Inf"
	case math.IsInf(f, -1):
		return "js.NegInf"
	case math.IsNaN(f):
		return "js.NaN"
	case f == 0 && math.Signbit(f):
		return "js.NegZero"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return "float64(" + s + ")"
}

func goLiteral(v js.Value) string {
	switch x := v.(type) {
	case string:
		return quote(x)
	case float64:
		return goNumber(x)
	case bool:
		return strconv.FormatBool(x)
	}
	return "js.Null"
}

// cond is a Go boolean for a JavaScript condition.
func (c *Compiler) cond(s *scope, n *Node) string {
	switch n.Type {
	case "BinaryExpression":
		op := n.Str("operator")
		l, r := c.expr(s, n.Child("left")), c.expr(s, n.Child("right"))
		switch op {
		case "!=":
			return fmt.Sprintf("!js.LooseEquals(%s, %s)", l, r)
		case "!==":
			return fmt.Sprintf("!js.StrictEquals(%s, %s)", l, r)
		}
		if f, ok := compareOps[op]; ok {
			return fmt.Sprintf("%s(%s, %s)", f, l, r)
		}
	case "UnaryExpression":
		if n.Str("operator") == "!" {
			return "!(" + c.cond(s, n.Child("argument")) + ")"
		}
	case "LogicalExpression":
		switch n.Str("operator") {
		case "&&":
			return "(" + c.cond(s, n.Child("left")) + " && " + c.cond(s, n.Child("right")) + ")"
		case "||":
			return "(" + c.cond(s, n.Child("left")) + " || " + c.cond(s, n.Child("right")) + ")"
		}
	case "Literal":
		if v, ok := literalValue(n); ok {
			return strconv.FormatBool(js.Truthy(v))
		}
	}
	return "js.Truthy(" + c.expr(s, n) + ")"
}

// namedExpr compiles an expression that names an anonymous function or
// class, as `const f = () => {}` names it "f".
func (c *Compiler) namedExpr(s *scope, n *Node, name string) string {
	switch n.Type {
	case "ArrowFunctionExpression", "FunctionExpression":
		if n.Child("id") == nil {
			return c.function(s, n, name)
		}
	case "ClassExpression":
		if n.Child("id") == nil {
			return c.class(s, n, name)
		}
	}
	return c.expr(s, n)
}

// iife wraps statements that compute a value.
func iife(body string) string { return "func() js.Value {\n" + body + "}()" }

func (c *Compiler) expr(s *scope, n *Node) string {
	switch n.Type {
	case "Identifier":
		return c.ident(s, n)
	case "Literal":
		if n.Has("regex") {
			re := n.Child("regex")
			return fmt.Sprintf("js.RegExpLiteral(%s, %s)", quote(re.Str("pattern")), quote(re.Str("flags")))
		}
		v, ok := literalValue(n)
		if !ok {
			c.problem(n, "a BigInt literal")
			return "js.Fail(\"BigInt\")"
		}
		return goLiteral(v)
	case "TemplateLiteral":
		return c.template(s, n)
	case "TaggedTemplateExpression":
		q := n.Child("quasi")
		var cooked, raws []string
		for _, el := range q.List("quasis") {
			v := el.Child("value")
			if v.Has("cooked") {
				cv, _ := js.ParseJSON(string(v.Raw("cooked")))
				cooked = append(cooked, quote(js.ToString(cv)))
			} else {
				cooked = append(cooked, "js.Undefined")
			}
			rv, _ := js.ParseJSON(string(v.Raw("raw")))
			raws = append(raws, quote(js.ToString(rv)))
		}
		args := []string{fmt.Sprintf("js.TemplateObject(%d, []js.Value{%s}, []string{%s})", c.site(), join(cooked), join(raws))}
		for _, e := range q.List("expressions") {
			args = append(args, c.expr(s, e))
		}
		return c.callWith(s, n.Child("tag"), args, false)
	case "ThisExpression":
		return c.thisExpr(s, n)
	case "ArrayExpression":
		return c.array(s, n.List("elements"))
	case "ObjectExpression":
		return c.object(s, n)
	case "FunctionExpression", "ArrowFunctionExpression":
		name := ""
		if id := n.Child("id"); id != nil {
			name = id.Str("name")
		}
		return c.function(s, n, name)
	case "ClassExpression":
		name := ""
		if id := n.Child("id"); id != nil {
			name = id.Str("name")
		}
		return c.class(s, n, name)
	case "UnaryExpression":
		return c.unary(s, n)
	case "UpdateExpression":
		return c.update(s, n)
	case "BinaryExpression":
		op := n.Str("operator")
		if f, ok := binaryOps[op]; ok {
			return fmt.Sprintf("%s(%s, %s)", f, c.expr(s, n.Child("left")), c.expr(s, n.Child("right")))
		}
		if n.Child("left").Type == "PrivateIdentifier" {
			return fmt.Sprintf("js.PrivateIn(%s, %s)", c.privateKey(s, n.Child("left")), c.expr(s, n.Child("right")))
		}
		return "js.Value(" + c.cond(s, n) + ")"
	case "LogicalExpression":
		t := c.temp("t")
		l, r := c.expr(s, n.Child("left")), c.expr(s, n.Child("right"))
		switch n.Str("operator") {
		case "&&":
			return iife(fmt.Sprintf("%s := %s\nif !js.Truthy(%s) {\nreturn %s\n}\nreturn %s\n", t, l, t, t, r))
		case "||":
			return iife(fmt.Sprintf("%s := %s\nif js.Truthy(%s) {\nreturn %s\n}\nreturn %s\n", t, l, t, t, r))
		default: // ??
			return iife(fmt.Sprintf("%s := %s\nif !js.IsNullish(%s) {\nreturn %s\n}\nreturn %s\n", t, l, t, t, r))
		}
	case "ConditionalExpression":
		return iife(fmt.Sprintf("if %s {\nreturn %s\n}\nreturn %s\n", c.cond(s, n.Child("test")), c.expr(s, n.Child("consequent")), c.expr(s, n.Child("alternate"))))
	case "AssignmentExpression":
		w := &writer{}
		v := c.assign(w, s, n)
		return iife(w.String() + "return " + v + "\n")
	case "SequenceExpression":
		w := &writer{}
		exprs := n.List("expressions")
		for _, e := range exprs[:len(exprs)-1] {
			c.exprStatement(w, s, e)
		}
		return iife(w.String() + "return " + c.expr(s, exprs[len(exprs)-1]) + "\n")
	case "MemberExpression":
		if n.Child("object").Type == "Super" {
			return fmt.Sprintf("js.SuperGet(%s, %s, %s)", s.fn.superProtoExpr(c, n), c.memberKey(s, n), c.thisExpr(s, n))
		}
		if n.Child("property").Type == "PrivateIdentifier" {
			return fmt.Sprintf("js.PrivateGet(%s, %s)", c.expr(s, n.Child("object")), c.privateKey(s, n.Child("property")))
		}
		if e, _, ok := c.namespaceMember(s, n); ok {
			return e
		}
		obj := c.expr(s, n.Child("object"))
		if !n.Bool("computed") {
			return fmt.Sprintf("js.GetIC(%s, %s, %s)", c.ic(), obj, quote(n.Child("property").Str("name")))
		}
		p := n.Child("property")
		if p.Type == "Literal" {
			if v, ok := literalValue(p); ok {
				if str, ok := v.(string); ok {
					return fmt.Sprintf("js.GetIC(%s, %s, %s)", c.ic(), obj, quote(str))
				}
			}
		}
		return fmt.Sprintf("js.GetV(%s, %s)", obj, c.expr(s, p))
	case "ChainExpression":
		w := &writer{}
		v := c.chain(w, s, n.Child("expression"))
		return iife(w.String() + "return " + v + "\n")
	case "CallExpression":
		return c.call(s, n)
	case "NewExpression":
		args := c.args(s, n.List("arguments"))
		return fmt.Sprintf("js.Construct(%s%s)", c.expr(s, n.Child("callee")), args)
	case "YieldExpression":
		g := s.fn.generator
		if g == "" {
			c.problem(n, "yield outside a generator")
			return "js.Fail(\"yield\")"
		}
		v := "js.Undefined"
		if a := n.Child("argument"); a != nil {
			v = c.expr(s, a)
		}
		if n.Bool("delegate") {
			return fmt.Sprintf("%s.YieldStar(%s)", g, v)
		}
		return fmt.Sprintf("%s.Yield(%s)", g, v)
	case "AwaitExpression":
		c.problem(n, "%s", strings.TrimSuffix(n.Type, "Expression"))
		return "js.Fail(" + quote(n.Type) + ")"
	case "ImportExpression":
		c.problem(n, "dynamic import()")
		return "js.Fail(\"import()\")"
	case "MetaProperty":
		c.problem(n, "%s.%s", n.Child("meta").Str("name"), n.Child("property").Str("name"))
		return "js.Fail(\"meta property\")"
	case "ParenthesizedExpression":
		return c.expr(s, n.Child("expression"))
	}
	c.problem(n, "the %s expression", n.Type)
	return "js.Fail(" + quote(n.Type) + ")"
}

// globalRead reads an undeclared name from globalThis, with a cache.
func (c *Compiler) globalRead(name string) string {
	c.globalRefs++
	return fmt.Sprintf("js.GlobalGetCached(&globals[%d], %s)", c.globalRefs-1, quote(name))
}

// ic allocates an inline cache for one property access.
func (c *Compiler) ic() string {
	c.ics++
	return fmt.Sprintf("&ics[%d]", c.ics-1)
}

func (c *Compiler) site() int {
	c.tmp++
	return c.tmp
}

func (c *Compiler) template(s *scope, n *Node) string {
	quasis := n.List("quasis")
	exprs := n.List("expressions")
	var parts []string
	for i, q := range quasis {
		cv, _ := js.ParseJSON(string(q.Child("value").Raw("cooked")))
		if str := js.ToString(cv); str != "" {
			parts = append(parts, quote(str))
		}
		if i < len(exprs) {
			parts = append(parts, "js.ToString("+c.expr(s, exprs[i])+")")
		}
	}
	if len(parts) == 0 {
		return `""`
	}
	return "js.Value(" + strings.Join(parts, " + ") + ")"
}

func (c *Compiler) ident(s *scope, n *Node) string {
	name := n.Str("name")
	if name == "undefined" && s.lookup(name) == nil {
		return "js.Undefined"
	}
	if name == "arguments" && s.lookup(name) == nil {
		f := s.fn
		for f != nil && f.arrow {
			f = f.parent
		}
		if f == nil || f.module {
			c.problem(n, "arguments outside a function")
			return "js.Undefined"
		}
		f.usesArguments = true
		return f.argumentsVar
	}
	if b := s.lookup(name); b != nil {
		return b.expr
	}
	return c.globalRead(name)
}

func (c *Compiler) thisExpr(s *scope, _ *Node) string {
	f := s.fn
	for f != nil && f.arrow {
		f = f.parent
	}
	if f == nil || f.module {
		return "js.Undefined"
	}
	if f.thisVar != "" {
		return fmt.Sprintf("js.CheckThis(%s)", f.thisVar)
	}
	return f.this
}

func (c *Compiler) array(s *scope, elems []*Node) string {
	var parts []string
	spread := false
	for _, e := range elems {
		switch {
		case e == nil:
			parts = append(parts, "js.Undefined")
		case e.Type == "SpreadElement":
			spread = true
			parts = append(parts, "...")
		default:
			parts = append(parts, c.expr(s, e))
		}
	}
	if !spread {
		return "js.Value(js.NewArray([]js.Value{" + join(parts) + "}))"
	}
	expr := "[]js.Value{}"
	for i, e := range elems {
		if e != nil && e.Type == "SpreadElement" {
			expr = fmt.Sprintf("append(%s, js.Spread(%s)...)", expr, c.expr(s, e.Child("argument")))
		} else {
			expr = fmt.Sprintf("append(%s, %s)", expr, parts[i])
		}
	}
	return "js.Value(js.NewArray(" + expr + "))"
}

func propKey(c *Compiler, s *scope, p *Node) (string, string) {
	k := p.Child("key")
	if p.Bool("computed") {
		return fmt.Sprintf("js.ToPropertyKey(%s)", c.expr(s, k)), ""
	}
	switch k.Type {
	case "Identifier":
		return quote(k.Str("name")), k.Str("name")
	case "Literal":
		v, _ := literalValue(k)
		ks := js.ToString(v)
		return quote(ks), ks
	case "PrivateIdentifier":
		return "", "#" + k.Str("name")
	}
	return quote(""), ""
}

func (c *Compiler) object(s *scope, n *Node) string {
	props := n.List("properties")
	simple := true
	for _, p := range props {
		if p.Type != "Property" || p.Str("kind") != "init" || p.Bool("computed") || p.Bool("method") {
			simple = false
			break
		}
		if _, name := propKey(c, s, p); name == "__proto__" {
			simple = false
			break
		}
	}
	if simple {
		var kv []string
		for _, p := range props {
			key, name := propKey(c, s, p)
			kv = append(kv, key, c.namedExpr(s, p.Child("value"), name))
		}
		return "js.Value(js.Obj(" + join(kv) + "))"
	}
	w := &writer{}
	o := c.temp("o")
	w.line("%s := js.NewPlainObject()", o)
	for _, p := range props {
		if p.Type == "SpreadElement" {
			w.line("js.CopyDataProperties(%s, %s)", o, c.expr(s, p.Child("argument")))
			continue
		}
		key, name := propKey(c, s, p)
		if name == "__proto__" && !p.Bool("computed") && !p.Bool("shorthand") && p.Str("kind") == "init" && !p.Bool("method") {
			w.line("js.SetProtoFromLiteral(%s, %s)", o, c.expr(s, p.Child("value")))
			continue
		}
		val := p.Child("value")
		switch p.Str("kind") {
		case "get", "set":
			k := c.temp("k")
			w.line("%s := %s", k, key)
			fn := c.methodFunction(s, val, p.Str("kind")+" "+name, o, "")
			if p.Str("kind") == "get" {
				w.line("%s.DefineAccessor(%s, js.AsObject(%s), nil, true)", o, k, fn)
			} else {
				w.line("%s.DefineAccessor(%s, nil, js.AsObject(%s), true)", o, k, fn)
			}
		default:
			if p.Bool("method") {
				w.line("%s.Define(%s, %s)", o, key, c.methodFunction(s, val, name, o, ""))
			} else {
				w.line("%s.Define(%s, %s)", o, key, c.namedExpr(s, val, name))
			}
		}
	}
	return iife(w.String() + "return " + o + "\n")
}

func (c *Compiler) unary(s *scope, n *Node) string {
	arg := n.Child("argument")
	switch op := n.Str("operator"); op {
	case "!":
		return "js.Value(!" + c.cond(s, arg) + ")"
	case "-":
		if arg.Type == "Literal" {
			if v, ok := literalValue(arg); ok {
				if f, ok := v.(float64); ok {
					return goNumber(-f)
				}
			}
		}
		return "js.Neg(" + c.expr(s, arg) + ")"
	case "+":
		return "js.Plus(" + c.expr(s, arg) + ")"
	case "~":
		return "js.BitNot(" + c.expr(s, arg) + ")"
	case "typeof":
		if arg.Type == "Identifier" && s.lookup(arg.Str("name")) == nil && arg.Str("name") != "undefined" && arg.Str("name") != "arguments" {
			return fmt.Sprintf("js.Value(js.TypeofGlobal(%s))", quote(arg.Str("name")))
		}
		return "js.Value(js.Typeof(" + c.expr(s, arg) + "))"
	case "void":
		if arg.Type == "Literal" {
			return "js.Undefined"
		}
		return iife("_ = " + c.expr(s, arg) + "\nreturn js.Undefined\n")
	case "delete":
		switch arg.Type {
		case "MemberExpression":
			obj := c.expr(s, arg.Child("object"))
			return fmt.Sprintf("js.Value(js.Delete(%s, %s))", obj, c.memberKey(s, arg))
		case "ChainExpression":
			c.problem(n, "delete of an optional chain")
			return "js.Fail(\"delete ?.\")"
		}
		return iife("_ = " + c.expr(s, arg) + "\nreturn true\n")
	}
	c.problem(n, "the %s operator", n.Str("operator"))
	return "js.Fail(\"unary\")"
}

// memberKey is a member expression's property key, as a Go PropKey.
func (c *Compiler) memberKey(s *scope, n *Node) string {
	if !n.Bool("computed") {
		return quote(n.Child("property").Str("name"))
	}
	return "js.ToPropertyKey(" + c.expr(s, n.Child("property")) + ")"
}

func (c *Compiler) privateKey(s *scope, n *Node) string {
	name := "#" + n.Str("name")
	if b := s.lookup(name); b != nil {
		return b.expr
	}
	c.problem(n, "the private name %s outside its class", name)
	return "js.UnknownPrivate"
}

// exprStatement compiles an expression for its effect.
func (c *Compiler) exprStatement(w *writer, s *scope, n *Node) {
	switch n.Type {
	case "AssignmentExpression":
		c.assign(w, s, n)
		return
	case "UpdateExpression":
		c.updateStatement(w, s, n)
		return
	case "SequenceExpression":
		for _, e := range n.List("expressions") {
			c.exprStatement(w, s, e)
		}
		return
	case "CallExpression":
		w.line("_ = %s", c.call(s, n))
		return
	case "Literal":
		return // "use strict" and other directives
	}
	w.line("_ = %s", c.expr(s, n))
}
