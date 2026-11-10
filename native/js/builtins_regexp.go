package js

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dlclark/regexp2/v2"
)

// RegExpPrototype is RegExp.prototype.
var RegExpPrototype = &Object{proto: ObjectPrototype}

type regexpData struct {
	re     *regexp2.Regexp
	source string
	flags  string
}

// NewRegExp compiles a regular expression literal or new RegExp(source, flags).
func NewRegExp(source, flags string) *Object {
	opts := regexp2.ECMAScript
	for _, f := range flags {
		switch f {
		case 'i':
			opts |= regexp2.IgnoreCase
		case 'm':
			opts |= regexp2.Multiline
		case 's':
			opts |= regexp2.Singleline
		case 'u':
			opts |= regexp2.Unicode
		case 'g', 'y', 'd':
		default:
			panic(newError(SyntaxError, "Invalid regular expression flags"))
		}
	}
	if strings.ContainsRune(flags, 's') {
		opts &^= regexp2.ECMAScript // regexp2's ECMAScript mode has no dotAll
	}
	re, err := regexp2.Compile(source, opts)
	if err != nil {
		panic(newError(SyntaxError, "Invalid regular expression: /"+source+"/: "+err.Error()))
	}
	o := NewObject(RegExpPrototype)
	o.Class, o.Internal = "RegExp", &regexpData{re: re, source: source, flags: sortFlags(flags)}
	o.add(prop{key: "lastIndex", value: 0.0, writable: true})
	return o
}

func sortFlags(f string) string {
	var b strings.Builder
	for _, c := range "dgimsuvy" {
		if strings.ContainsRune(f, c) {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func (o *Object) regexpFlags() string { return o.Internal.(*regexpData).flags }

func toRegExp(v Value, flags string) *Object {
	if o, ok := v.(*Object); ok && o.Class == "RegExp" {
		return o
	}
	src := "(?:)"
	if v != Undefined {
		src = regexpEscapeSource(ToString(v))
	}
	return NewRegExp(src, flags)
}

// regexpEscapeSource keeps a string's meaning as a pattern: match("a.b")
// treats the string as a regexp source, so nothing is escaped.
func regexpEscapeSource(s string) string { return s }

// utf16Offset converts a rune index into s to a UTF-16 index.
func utf16Offset(s string, runeIndex int) int {
	if isASCII(s) {
		return runeIndex
	}
	n, r := 0, 0
	for _, c := range s {
		if r == runeIndex {
			break
		}
		if c >= 0x10000 {
			n += 2
		} else {
			n++
		}
		r++
	}
	return n
}

// runeOffset converts a UTF-16 index into s to a rune index.
func runeOffset(s string, unitIndex int) int {
	if isASCII(s) {
		return unitIndex
	}
	n, r := 0, 0
	for _, c := range s {
		if n >= unitIndex {
			break
		}
		if c >= 0x10000 {
			n += 2
		} else {
			n++
		}
		r++
	}
	return r
}

type match struct {
	index  int // UTF-16
	end    int
	groups []Value // the whole match, then each group (undefined if it didn't take part)
	names  map[string]Value
}

// exec runs the regexp once, honoring lastIndex for g and y.
func exec(r *Object, s string) *match {
	d := r.Internal.(*regexpData)
	global := strings.ContainsAny(d.flags, "gy")
	start := 0
	if global {
		start = int(ToIntegerOrInfinity(r.Get("lastIndex")))
		if start > Len16(s) {
			r.Set("lastIndex", 0.0)
			return nil
		}
	}
	m, err := d.re.FindStringMatchStartingAt(s, runeOffset(s, start))
	if err != nil || m == nil || (strings.ContainsRune(d.flags, 'y') && utf16Offset(s, m.RuneIndex) != start) {
		if global {
			r.Set("lastIndex", 0.0)
		}
		return nil
	}
	res := &match{index: utf16Offset(s, m.RuneIndex)}
	res.end = res.index + Len16(m.String())
	gs := m.Groups()
	for _, g := range gs {
		if len(g.Captures) == 0 {
			res.groups = append(res.groups, Undefined)
		} else {
			res.groups = append(res.groups, g.String())
		}
	}
	for _, name := range d.re.GetGroupNames() {
		if _, err := strconv.Atoi(name); err == nil {
			continue
		}
		if res.names == nil {
			res.names = map[string]Value{}
		}
		if g := m.GroupByName(name); g != nil && len(g.Captures) > 0 {
			res.names[name] = g.String()
		} else {
			res.names[name] = Undefined
		}
	}
	if global {
		r.Set("lastIndex", float64(res.end))
	}
	return res
}

func (m *match) array(s string) *Object {
	a := NewArray(append([]Value(nil), m.groups...))
	a.Define("index", float64(m.index))
	a.Define("input", s)
	if m.names != nil {
		g := NewObject(nil)
		for _, k := range sortedNames(m.names) {
			g.Define(k, m.names[k])
		}
		a.Define("groups", g)
	} else {
		a.Define("groups", Undefined)
	}
	return a
}

func sortedNames(m map[string]Value) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Group order is the order they appear in the pattern; regexp2 gives
	// them in that order, so keep it stable by name as a fallback.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func setupRegExp() {
	ctor := NewConstructor("RegExp", 2, nil, func(args []Value, nt *Object) Value {
		pat, flags := Arg(args, 0), Arg(args, 1)
		src := "(?:)"
		fl := ""
		if o, ok := pat.(*Object); ok && o.Class == "RegExp" {
			d := o.Internal.(*regexpData)
			src, fl = d.source, d.flags
		} else if pat != Undefined {
			src = ToString(pat)
		}
		if flags != Undefined {
			fl = ToString(flags)
		}
		return NewRegExp(src, fl)
	}, RegExpPrototype)
	ctor.call = func(_ Value, args []Value) Value { return ctor.construct(args, ctor) }
	Global.DefineHidden("RegExp", ctor)
	p := RegExpPrototype
	this := func(v Value) *Object {
		if o, ok := v.(*Object); ok && o.Class == "RegExp" {
			return o
		}
		panic(TypeErrorf("RegExp method called on incompatible receiver"))
	}
	method(p, "exec", 1, func(t Value, args []Value) Value {
		r := this(t)
		s := ToString(Arg(args, 0))
		m := exec(r, s)
		if m == nil {
			return Null
		}
		return m.array(s)
	})
	method(p, "test", 1, func(t Value, args []Value) Value { return exec(this(t), ToString(Arg(args, 0))) != nil })
	method(p, "toString", 0, func(t Value, _ []Value) Value {
		d := this(t).Internal.(*regexpData)
		return "/" + d.source + "/" + d.flags
	})
	getter := func(name string, fn func(d *regexpData) Value) {
		p.DefineAccessor(name, NewFunction("get "+name, 0, func(t Value, _ []Value) Value {
			if t == Value(RegExpPrototype) {
				return Undefined
			}
			return fn(this(t).Internal.(*regexpData))
		}), nil, false)
	}
	getter("source", func(d *regexpData) Value { return d.source })
	getter("flags", func(d *regexpData) Value { return d.flags })
	for name, f := range map[string]rune{"global": 'g', "ignoreCase": 'i', "multiline": 'm', "dotAll": 's', "unicode": 'u', "sticky": 'y', "hasIndices": 'd'} {
		getter(name, func(d *regexpData) Value { return strings.ContainsRune(d.flags, f) })
	}
}

func regexpMatch(r *Object, s string) Value {
	if !strings.ContainsRune(r.regexpFlags(), 'g') {
		m := exec(r, s)
		if m == nil {
			return Null
		}
		return m.array(s)
	}
	r.Set("lastIndex", 0.0)
	var out []Value
	for {
		m := exec(r, s)
		if m == nil {
			break
		}
		out = append(out, m.groups[0])
		if m.end == m.index {
			r.Set("lastIndex", float64(advance(s, m.end, strings.ContainsRune(r.regexpFlags(), 'u'))))
		}
	}
	if out == nil {
		return Null
	}
	return NewArray(out)
}

func regexpMatchAll(r *Object, s string) Value {
	clone := NewRegExp(r.Internal.(*regexpData).source, r.regexpFlags())
	clone.Set("lastIndex", r.Get("lastIndex"))
	done := false
	return NewIterator(func() (Value, bool) {
		if done {
			return Undefined, false
		}
		m := exec(clone, s)
		if m == nil {
			done = true
			return Undefined, false
		}
		if m.end == m.index {
			clone.Set("lastIndex", float64(advance(s, m.end, strings.ContainsRune(clone.regexpFlags(), 'u'))))
		}
		return m.array(s), true
	})
}

func regexpSearch(r *Object, s string) Value {
	saved := r.Get("lastIndex")
	r.Set("lastIndex", 0.0)
	d := r.Internal.(*regexpData)
	m, err := d.re.FindStringMatch(s)
	r.Set("lastIndex", saved)
	if err != nil || m == nil {
		return -1.0
	}
	return float64(utf16Offset(s, m.RuneIndex))
}

// advance moves past an empty match by one code unit, or one code point
// with the u flag.
func advance(s string, i int, unicode bool) int {
	if unicode {
		if c := UnitAt(s, i); c >= 0xD800 && c <= 0xDBFF {
			if d := UnitAt(s, i+1); d >= 0xDC00 && d <= 0xDFFF {
				return i + 2
			}
		}
	}
	return i + 1
}

func regexpSplit(r *Object, s string, limitArg Value) Value {
	limit := 1<<32 - 1
	if limitArg != Undefined {
		limit = int(ToUint32(limitArg))
	}
	var out []Value
	if limit == 0 {
		return NewArray(nil)
	}
	d := r.Internal.(*regexpData)
	if s == "" {
		if m, _ := d.re.FindStringMatch(s); m != nil {
			return NewArray(nil)
		}
		return NewArray([]Value{s})
	}
	size := Len16(s)
	p, q := 0, 0
	for q < size {
		m, err := d.re.FindStringMatchStartingAt(s, runeOffset(s, q))
		if err != nil || m == nil {
			break
		}
		start := utf16Offset(s, m.RuneIndex)
		end := start + Len16(m.String())
		if end == start && start == p || start >= size {
			if start >= size {
				break
			}
			q = start + 1
			if end == start && start == p {
				continue
			}
		}
		if end == p {
			q = start + 1
			continue
		}
		out = append(out, Slice16(s, p, start))
		if len(out) == limit {
			return NewArray(out)
		}
		for _, g := range m.Groups()[1:] {
			if len(g.Captures) == 0 {
				out = append(out, Undefined)
			} else {
				out = append(out, g.String())
			}
			if len(out) == limit {
				return NewArray(out)
			}
		}
		p = end
		q = end
		if end == start {
			q = end + 1
		}
	}
	out = append(out, Slice16(s, p, size))
	return NewArray(out)
}

// replace is String.prototype.replace and replaceAll.
func replace(s string, pattern, replacement Value, all bool) Value {
	fn, isFn := replacement.(*Object)
	isFn = isFn && fn.call != nil
	var repl string
	if !isFn {
		repl = ToString(replacement)
	}
	var b strings.Builder
	if r, ok := pattern.(*Object); ok && r.Class == "RegExp" {
		global := strings.ContainsRune(r.regexpFlags(), 'g')
		if all && !global {
			panic(TypeErrorf("replaceAll must be called with a global RegExp"))
		}
		if global {
			r.Set("lastIndex", 0.0)
		}
		last := 0
		for {
			m := exec(r, s)
			if m == nil {
				break
			}
			b.WriteString(Slice16(s, last, m.index))
			if isFn {
				args := append([]Value(nil), m.groups...)
				args = append(args, float64(m.index), s)
				if m.names != nil {
					g := NewObject(nil)
					for _, k := range sortedNames(m.names) {
						g.Define(k, m.names[k])
					}
					args = append(args, g)
				}
				b.WriteString(ToString(fn.call(Undefined, args)))
			} else {
				b.WriteString(expandReplacement(repl, s, m))
			}
			last = m.end
			if !global {
				break
			}
			if m.end == m.index {
				r.Set("lastIndex", float64(advance(s, m.end, strings.ContainsRune(r.regexpFlags(), 'u'))))
			}
		}
		b.WriteString(Slice16(s, last, Len16(s)))
		return b.String()
	}
	pat := ToString(pattern)
	last := 0
	from := 0
	for {
		i := Index16(s, pat, from)
		if i < 0 {
			break
		}
		b.WriteString(Slice16(s, last, i))
		m := &match{index: i, end: i + Len16(pat), groups: []Value{pat}}
		if isFn {
			b.WriteString(ToString(fn.call(Undefined, []Value{pat, float64(i), s})))
		} else {
			b.WriteString(expandReplacement(repl, s, m))
		}
		last = m.end
		if !all {
			break
		}
		from = m.end
		if pat == "" {
			from++
			if from > Len16(s) {
				break
			}
		}
	}
	b.WriteString(Slice16(s, last, Len16(s)))
	return b.String()
}

// expandReplacement applies $$, $&, $`, $', $n and $<name>.
func expandReplacement(repl, s string, m *match) string {
	if !strings.Contains(repl, "$") {
		return repl
	}
	var b strings.Builder
	for i := 0; i < len(repl); i++ {
		c := repl[i]
		if c != '$' || i+1 >= len(repl) {
			b.WriteByte(c)
			continue
		}
		n := repl[i+1]
		switch {
		case n == '$':
			b.WriteByte('$')
			i++
		case n == '&':
			b.WriteString(ToString(m.groups[0]))
			i++
		case n == '`':
			b.WriteString(Slice16(s, 0, m.index))
			i++
		case n == '\'':
			b.WriteString(Slice16(s, m.end, Len16(s)))
			i++
		case n >= '0' && n <= '9':
			// One or two digits, preferring two if that group exists.
			idx := int(n - '0')
			width := 1
			if i+2 < len(repl) && repl[i+2] >= '0' && repl[i+2] <= '9' {
				two := idx*10 + int(repl[i+2]-'0')
				if two >= 1 && two < len(m.groups) {
					idx, width = two, 2
				}
			}
			if idx >= 1 && idx < len(m.groups) {
				if g := m.groups[idx]; g != Undefined {
					b.WriteString(ToString(g))
				}
				i += width
			} else {
				b.WriteByte('$')
			}
		case n == '<' && m.names != nil:
			end := strings.IndexByte(repl[i:], '>')
			if end < 0 {
				b.WriteByte('$')
				continue
			}
			name := repl[i+2 : i+end]
			if v, ok := m.names[name]; ok && v != Undefined {
				b.WriteString(ToString(v))
			}
			i += end
		default:
			b.WriteByte('$')
		}
	}
	return b.String()
}

var _ = utf8.RuneSelf
